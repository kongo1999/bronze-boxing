package server

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

type inventoryHandler struct{ store *db.Store }

func registerInventory(r fiber.Router, store *db.Store) {
	h := &inventoryHandler{store}
	g := r.Group("/inventory")
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.remove)
	g.Post("/:id/sell", h.sell)
	r.Get("/sales", h.sales)
}

type inventoryInput struct {
	Name              string  `json:"name"`
	SKU               string  `json:"sku"`
	Stock             int     `json:"stock"`
	Price             float64 `json:"price"`
	CostPrice         float64 `json:"costPrice"`
	LowStockThreshold int     `json:"lowStockThreshold"`
	Active            *bool   `json:"active"`
}

func (h *inventoryHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	cur, err := h.store.Coll(models.CollInventory).
		Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return err
	}
	out := []models.InventoryItem{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

func (h *inventoryHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in inventoryInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name is required")
	}
	now := time.Now()
	item := models.InventoryItem{
		Name:              in.Name,
		SKU:               in.SKU,
		Stock:             in.Stock,
		Price:             in.Price,
		CostPrice:         in.CostPrice,
		LowStockThreshold: in.LowStockThreshold,
		Active:            in.Active == nil || *in.Active,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	res, err := h.store.Coll(models.CollInventory).InsertOne(ctx, item)
	if err != nil {
		return err
	}
	item.ID = res.InsertedID.(primitive.ObjectID)
	return c.Status(fiber.StatusCreated).JSON(item)
}

func (h *inventoryHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in inventoryInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	set := bson.M{
		"name":              strings.TrimSpace(in.Name),
		"sku":               in.SKU,
		"stock":             in.Stock,
		"price":             in.Price,
		"costPrice":         in.CostPrice,
		"lowStockThreshold": in.LowStockThreshold,
		"updatedAt":         time.Now(),
	}
	if in.Active != nil {
		set["active"] = *in.Active
	}
	if _, err := h.store.Coll(models.CollInventory).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return err
	}
	var item models.InventoryItem
	if err := h.store.Coll(models.CollInventory).FindOne(ctx, bson.M{"_id": id}).Decode(&item); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}
	return c.JSON(item)
}

func (h *inventoryHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := h.store.Coll(models.CollInventory).DeleteOne(ctx, bson.M{"_id": id}); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}

type sellInput struct {
	Qty          int      `json:"qty"`
	Trainee      string   `json:"trainee"`
	RecordIncome bool     `json:"recordIncome"`
	UnitPrice    *float64 `json:"unitPrice"`
}

// sell records a sale: decrements stock, logs who bought what, and optionally
// records the sale as cash income (a payment of type "sale").
func (h *inventoryHandler) sell(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in sellInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if in.Qty <= 0 {
		in.Qty = 1
	}
	var item models.InventoryItem
	if err := h.store.Coll(models.CollInventory).FindOne(ctx, bson.M{"_id": id}).Decode(&item); err != nil {
		return fiber.NewError(fiber.StatusNotFound, "item not found")
	}
	if item.Stock < in.Qty {
		return fiber.NewError(fiber.StatusBadRequest, "not enough stock")
	}
	unit := item.Price
	if in.UnitPrice != nil {
		unit = *in.UnitPrice
	}
	now := time.Now()
	sale := models.Sale{
		Item:      item.ID,
		ItemName:  item.Name,
		Qty:       in.Qty,
		UnitPrice: unit,
		Total:     unit * float64(in.Qty),
		Date:      now,
		CreatedAt: now,
	}
	if in.Trainee != "" {
		if tid, err := primitive.ObjectIDFromHex(in.Trainee); err == nil {
			sale.Trainee = &tid
			names := traineeNames(ctx, h.store, []primitive.ObjectID{tid})
			sale.TraineeName = names[tid]
		}
	}

	// Decrement stock.
	if _, err := h.store.Coll(models.CollInventory).UpdateOne(ctx, bson.M{"_id": item.ID},
		bson.M{"$inc": bson.M{"stock": -in.Qty}, "$set": bson.M{"updatedAt": now}}); err != nil {
		return err
	}

	res, err := h.store.Coll(models.CollSales).InsertOne(ctx, sale)
	if err != nil {
		return err
	}
	sale.ID = res.InsertedID.(primitive.ObjectID)

	if in.RecordIncome {
		pay := models.Payment{
			Trainee:     sale.Trainee,
			TraineeName: sale.TraineeName,
			Amount:      sale.Total,
			Type:        models.PaySale,
			Date:        now,
			Note:        sale.ItemName,
			SaleID:      &sale.ID,
			CreatedAt:   now,
		}
		if pres, err := h.store.Coll(models.CollPayments).InsertOne(ctx, pay); err == nil {
			pid := pres.InsertedID.(primitive.ObjectID)
			sale.PaymentID = &pid
			_, _ = h.store.Coll(models.CollSales).UpdateOne(ctx, bson.M{"_id": sale.ID},
				bson.M{"$set": bson.M{"paymentId": pid}})
		}
	}
	return c.Status(fiber.StatusCreated).JSON(sale)
}

func (h *inventoryHandler) sales(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	filter := bson.M{}
	if c.Query("from") != "" || c.Query("to") != "" {
		from, to := parseRange(c)
		filter["date"] = bson.M{"$gte": from, "$lte": to}
	}
	if t := c.Query("trainee"); t != "" {
		if tid, err := primitive.ObjectIDFromHex(t); err == nil {
			filter["trainee"] = tid
		}
	}
	cur, err := h.store.Coll(models.CollSales).
		Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "date", Value: -1}}))
	if err != nil {
		return err
	}
	out := []models.Sale{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}
