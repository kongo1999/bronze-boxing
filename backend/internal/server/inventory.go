package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.remove)
	g.Post("/:id/sell", h.sell)
	g.Get("/:id/movements", h.movements)
	g.Post("/:id/adjustments", h.adjust)
	g.Post("/:id/archive", h.setActive(false))
	g.Post("/:id/unarchive", h.setActive(true))
	r.Get("/sales", h.sales)
	r.Get("/sales/:id", h.getSale)
	r.Get("/sales/:id/receipt", h.saleReceipt)
	r.Get("/sales/:id/returns", h.listReturns)
	r.Post("/sales/:id/returns", h.returnSale)
	r.Put("/sales/:id", h.correctSale)        // correct qty / buyer (adjusts stock)
	r.Post("/sales/:id/void", h.voidSalePost) // void a sale (restocks)
	r.Delete("/sales/:id", h.voidSale)        // legacy spelling of void
}

type inventoryInput struct {
	Name              string  `json:"name"`
	SKU               string  `json:"sku"`
	Stock             *int    `json:"stock"`
	Price             float64 `json:"price"`
	CostPrice         float64 `json:"costPrice"`
	LowStockThreshold int     `json:"lowStockThreshold"`
	Active            *bool   `json:"active"`
	Reason            string  `json:"reason"` // required when a PUT changes stock
}

// list returns the shop's items by name. Filters: ?active=true|false,
// ?stock=short (low or out) | low | out.
func (h *inventoryHandler) list(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	filter := bson.M{}
	switch c.Query("active") {
	case "":
	case "true":
		filter["active"] = bson.M{"$ne": false}
	case "false":
		filter["active"] = false
	default:
		return badField("active", "active must be true or false")
	}
	stock := c.Query("stock")
	if stock != "" {
		if err := oneOf("stock", stock, "short", "low", "out"); err != nil {
			return err
		}
	}
	cur, err := h.store.Coll(models.CollInventory).
		Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return err
	}
	all := []models.InventoryItem{}
	if err := cur.All(ctx, &all); err != nil {
		return err
	}
	if stock == "" {
		return c.JSON(all)
	}
	out := []models.InventoryItem{}
	for _, it := range all {
		if s := it.Shortage(); s != "" && (stock == "short" || stock == s) {
			out = append(out, it)
		}
	}
	return c.JSON(out)
}

func findItem(ctx context.Context, store *db.Store, id primitive.ObjectID) (models.InventoryItem, error) {
	var it models.InventoryItem
	if err := store.Coll(models.CollInventory).FindOne(ctx, bson.M{"_id": id}).Decode(&it); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return it, apiErr(http.StatusNotFound, CodeItemNotFound, "item not found")
		}
		return it, err
	}
	return it, nil
}

func (h *inventoryHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	it, err := findItem(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(it)
}

func validateItem(in inventoryInput) (inventoryInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.SKU = strings.TrimSpace(in.SKU)
	if in.Name == "" {
		return in, badField("name", "name is required")
	}
	var err error
	if in.Price, err = validMoney("price", in.Price, true); err != nil {
		return in, err
	}
	if in.CostPrice, err = validMoney("costPrice", in.CostPrice, true); err != nil {
		return in, err
	}
	if in.Stock != nil && *in.Stock < 0 {
		return in, badField("stock", "stock can't be negative")
	}
	if in.LowStockThreshold < 0 {
		return in, badField("lowStockThreshold", "low-stock threshold can't be negative")
	}
	return in, nil
}

// recordMovement appends one line to an item's stock ledger (inside the
// caller's transaction, right after the stock change it explains).
func recordMovement(ctx context.Context, store *db.Store, m models.StockMovement) error {
	if m.At.IsZero() {
		m.At = time.Now()
	}
	_, err := store.Coll(models.CollMovements).InsertOne(ctx, m)
	return err
}

func (h *inventoryHandler) create(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in inventoryInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	in, err := validateItem(in)
	if err != nil {
		return err
	}
	now := time.Now()
	actor := actorOf(c)
	stock := 0
	if in.Stock != nil {
		stock = *in.Stock
	}
	item := models.InventoryItem{
		ID:                primitive.NewObjectID(),
		Name:              in.Name,
		SKU:               in.SKU,
		Stock:             stock,
		Price:             in.Price,
		CostPrice:         in.CostPrice,
		LowStockThreshold: in.LowStockThreshold,
		Active:            in.Active == nil || *in.Active,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		if _, err := h.store.Coll(models.CollInventory).InsertOne(tx, item); err != nil {
			return err
		}
		if err := recordMovement(tx, h.store, models.StockMovement{
			Item: item.ID, ItemName: item.Name, Delta: stock, Kind: models.MoveOpening,
			Reason: "Opening stock", StockAfter: stock, At: now, Actor: actor,
		}); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "item", Ref: item.ID, Action: "create", After: item, Actor: actor})
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(item)
}

// update edits an item's details. Stock is not a free-form field: a PUT that
// changes it is a guarded correction that must give a reason and is written
// to the stock ledger, so the ledger always adds up to what's on the shelf.
func (h *inventoryHandler) update(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in inventoryInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if in, err = validateItem(in); err != nil {
		return err
	}
	prev, err := findItem(ctx, h.store, id)
	if err != nil {
		return err
	}
	reason := strings.TrimSpace(in.Reason)
	stockChange := in.Stock != nil && *in.Stock != prev.Stock
	if stockChange {
		if reason, err = requireReason(in.Reason, "correct the stock count"); err != nil {
			return err
		}
	}
	actor := actorOf(c)
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		live, err := findItem(tx, h.store, id)
		if err != nil {
			return err
		}
		set := bson.M{
			"name":              in.Name,
			"sku":               in.SKU,
			"price":             in.Price,
			"costPrice":         in.CostPrice,
			"lowStockThreshold": in.LowStockThreshold,
			"updatedAt":         time.Now(),
		}
		if in.Active != nil {
			set["active"] = *in.Active
		}
		filter := bson.M{"_id": id}
		if stockChange {
			// Set the counted value, guarded on the stock we read so a sale
			// racing this correction can't be silently overwritten.
			filter["stock"] = live.Stock
			set["stock"] = *in.Stock
		}
		res, err := h.store.Coll(models.CollInventory).UpdateOne(tx, filter, bson.M{"$set": set})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return apiErr(http.StatusConflict, CodeConflict, "stock changed while you were editing — reload and try again")
		}
		if stockChange {
			if err := recordMovement(tx, h.store, models.StockMovement{
				Item: id, ItemName: in.Name, Delta: *in.Stock - live.Stock, Kind: models.MoveCorrection,
				Reason: reason, StockAfter: *in.Stock, Actor: actor,
			}); err != nil {
				return err
			}
		}
		after, err := findItem(tx, h.store, id)
		if err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "item", Ref: id, Action: "update",
			Before: live, After: after, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	item, err := findItem(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(item)
}

// remove deletes an item only while nothing has happened to it: no sales
// and no stock movement beyond its opening line (e.g. one added by
// mistake). An item with history is archived instead, so its stock ledger
// and old sales keep pointing at a real record.
func (h *inventoryHandler) remove(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	actor := actorOf(c)
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		it, err := findItem(tx, h.store, id)
		if err != nil {
			return err
		}
		sales, err := h.store.Coll(models.CollSales).CountDocuments(tx, bson.M{"item": id})
		if err != nil {
			return err
		}
		moves, err := h.store.Coll(models.CollMovements).CountDocuments(tx, bson.M{"item": id, "kind": bson.M{"$ne": models.MoveOpening}})
		if err != nil {
			return err
		}
		if sales > 0 || moves > 0 {
			return apiErr(http.StatusConflict, CodeConflict,
				fmt.Sprintf("%s has sales or stock history — archive it instead so that history stays linked", it.Name)).
				withDetails(map[string]int64{"sales": sales, "movements": moves})
		}
		if _, err := h.store.Coll(models.CollInventory).DeleteOne(tx, bson.M{"_id": id}); err != nil {
			return err
		}
		if _, err := h.store.Coll(models.CollMovements).DeleteMany(tx, bson.M{"item": id, "kind": models.MoveOpening}); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "item", Ref: id, Action: "delete", Before: it, Actor: actor})
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (h *inventoryHandler) movements(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	cur, err := h.store.Coll(models.CollMovements).Find(ctx, bson.M{"item": id},
		options.Find().SetSort(bson.D{{Key: "at", Value: -1}, {Key: "_id", Value: -1}}).
			SetLimit(int64(min(max(atoiDefault(c.Query("limit"), 200), 1), 1000))))
	if err != nil {
		return err
	}
	out := []models.StockMovement{}
	if err := cur.All(ctx, &out); err != nil {
		return err
	}
	return c.JSON(out)
}

type sellInput struct {
	Qty         int      `json:"qty"`
	Trainee     string   `json:"trainee"`
	UnitPrice   *float64 `json:"unitPrice"`
	PriceReason string   `json:"priceReason"` // required when unitPrice differs from the list price
	Method      string   `json:"method"`      // how the buyer paid; cash when empty
}

// takeStock atomically removes qty from an item — only if it is active and
// enough remains — and returns the stock left. Explains a refusal precisely.
func takeStock(ctx context.Context, store *db.Store, id primitive.ObjectID, qty int, requireActive bool) (models.InventoryItem, error) {
	filter := bson.M{"_id": id, "stock": bson.M{"$gte": qty}}
	if requireActive {
		filter["active"] = bson.M{"$ne": false}
	}
	var it models.InventoryItem
	err := store.Coll(models.CollInventory).FindOneAndUpdate(ctx, filter,
		bson.M{"$inc": bson.M{"stock": -qty}, "$set": bson.M{"updatedAt": time.Now()}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&it)
	if errors.Is(err, mongo.ErrNoDocuments) {
		cur, ferr := findItem(ctx, store, id)
		if ferr != nil {
			return it, ferr
		}
		if requireActive && !cur.Active {
			return it, apiErr(http.StatusConflict, CodeItemInactive, cur.Name+" is archived and can't be sold")
		}
		return it, apiErr(http.StatusConflict, CodeInsufficient,
			fmt.Sprintf("not enough stock: %d × %s left", cur.Stock, cur.Name)).
			withField("qty").withDetails(map[string]int{"available": cur.Stock})
	}
	return it, err
}

// sell records a sale: decrements stock and logs who bought what. The sale is
// shop income — it counts toward Financials (via the sales collection), but is
// NOT a "Money"/crew payment, so it stays out of the dues ledger. Stock, sale,
// ledger line and audit line commit together or not at all.
func (h *inventoryHandler) sell(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in sellInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if in.Qty <= 0 {
		return badField("qty", "quantity must be at least 1")
	}
	if in.Method != "" {
		if err := oneOf("method", in.Method, payMethods...); err != nil {
			return err
		}
	}
	item, err := findItem(ctx, h.store, id)
	if err != nil {
		return err
	}
	unit := round2(item.Price)
	priceReason := ""
	if in.UnitPrice != nil {
		if unit, err = validMoney("unitPrice", *in.UnitPrice, true); err != nil {
			return err
		}
		if models.Cents(unit) != models.Cents(item.Price) {
			if priceReason, err = requireReason(in.PriceReason, "sell at a different price"); err != nil {
				return err
			}
		}
	}
	now := time.Now()
	actor := actorOf(c)
	sale := models.Sale{
		ID:          primitive.NewObjectID(),
		Item:        item.ID,
		ItemName:    item.Name,
		Qty:         in.Qty,
		UnitPrice:   unit,
		UnitCost:    round2(item.CostPrice),
		ListPrice:   round2(item.Price),
		PriceReason: priceReason,
		Method:      in.Method,
		Total:       models.Amount(models.Cents(unit) * int64(in.Qty)),
		Date:        now,
		CreatedAt:   now,
		CreatedBy:   actor,
	}
	if in.Trainee != "" {
		t, err := loadTrainee(ctx, h.store, in.Trainee, "trainee")
		if err != nil {
			return err
		}
		sale.Trainee, sale.TraineeName = &t.ID, t.Name
	}
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		after, err := takeStock(tx, h.store, item.ID, in.Qty, true)
		if err != nil {
			return err
		}
		if err := failpoint("sell.afterStock"); err != nil {
			return err
		}
		if _, err := h.store.Coll(models.CollSales).InsertOne(tx, sale); err != nil {
			return err
		}
		if err := failpoint("sell.afterSale"); err != nil {
			return err
		}
		if err := recordMovement(tx, h.store, models.StockMovement{
			Item: item.ID, ItemName: item.Name, Delta: -in.Qty, Kind: models.MoveSale,
			RefType: "sale", Ref: &sale.ID, StockAfter: after.Stock, At: now, Actor: actor,
		}); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "sale", Ref: sale.ID, Action: "create", After: sale, Actor: actor, Reason: priceReason})
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(sale)
}

func (h *inventoryHandler) sales(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	filter := bson.M{}
	if c.Query("from") != "" || c.Query("to") != "" || c.Query("m") != "" {
		from, to, err := monthOrRange(c)
		if err != nil {
			return err
		}
		filter["date"] = bson.M{"$gte": from, "$lt": to}
	}
	if t := c.Query("trainee"); t != "" {
		tid, err := parseOID(t, "trainee")
		if err != nil {
			return err
		}
		filter["trainee"] = tid
	}
	if it := c.Query("item"); it != "" {
		iid, err := parseOID(it, "item")
		if err != nil {
			return err
		}
		filter["item"] = iid
	}
	if q, ok := searchQuery(c); ok {
		return rankedFind(c, ctx, h.store, models.CollSales, kindSale, filter, q,
			func(s models.Sale) primitive.ObjectID { return s.ID })
	}
	return pagedFind[models.Sale](c, ctx, h.store.Coll(models.CollSales), filter,
		bson.D{{Key: "date", Value: -1}, {Key: "_id", Value: -1}})
}

func findSale(ctx context.Context, store *db.Store, id primitive.ObjectID) (models.Sale, error) {
	var s models.Sale
	if err := store.Coll(models.CollSales).FindOne(ctx, bson.M{"_id": id}).Decode(&s); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return s, notFound("sale")
		}
		return s, err
	}
	return s, nil
}

// getSale returns a single sale by id (powers the sale detail page).
func (h *inventoryHandler) getSale(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	sale, err := findSale(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(sale)
}

func (h *inventoryHandler) voidSale(c *fiber.Ctx) error {
	return h.doVoidSale(c, c.Query("reason"))
}

func (h *inventoryHandler) voidSalePost(c *fiber.Ctx) error {
	var in struct {
		Reason string `json:"reason"`
	}
	_ = c.BodyParser(&in)
	return h.doVoidSale(c, in.Reason)
}

// doVoidSale reverses a sale the bookkeeping way: the record stays forever,
// marked void (so it no longer counts as shop income), and the units still
// out (sold minus already returned) go back into stock — one transaction, and
// the void flag is set conditionally so a double-void can never restock twice.
func (h *inventoryHandler) doVoidSale(c *fiber.Ctx, rawReason string) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	reason, err := requireReason(rawReason, "void a sale")
	if err != nil {
		return err
	}
	actor := actorOf(c)
	restocked := 0
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		restocked = 0
		sale, err := findSale(tx, h.store, id)
		if err != nil {
			return err
		}
		if sale.VoidedAt != nil {
			return apiErr(http.StatusConflict, CodeAlreadyVoided, "sale is already voided")
		}
		now := time.Now()
		upd, err := h.store.Coll(models.CollSales).UpdateOne(tx,
			bson.M{"_id": id, "voidedAt": nil},
			bson.M{"$set": bson.M{"voidedAt": now, "voidReason": reason, "voidedBy": actor}})
		if err != nil {
			return err
		}
		if upd.ModifiedCount == 0 {
			return apiErr(http.StatusConflict, CodeAlreadyVoided, "sale is already voided")
		}
		if err := failpoint("voidSale.afterFlag"); err != nil {
			return err
		}
		back := sale.Qty - sale.ReturnedQty
		if back > 0 {
			var it models.InventoryItem
			err := h.store.Coll(models.CollInventory).FindOneAndUpdate(tx, bson.M{"_id": sale.Item},
				bson.M{"$inc": bson.M{"stock": back}, "$set": bson.M{"updatedAt": now}},
				options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&it)
			switch {
			case errors.Is(err, mongo.ErrNoDocuments):
				// The item was removed from the shop: nothing to restock into.
			case err != nil:
				return err
			default:
				restocked = back
				if err := recordMovement(tx, h.store, models.StockMovement{
					Item: it.ID, ItemName: it.Name, Delta: back, Kind: models.MoveSaleVoid, Reason: reason,
					RefType: "sale", Ref: &sale.ID, StockAfter: it.Stock, At: now, Actor: actor,
				}); err != nil {
					return err
				}
			}
		}
		after := sale
		after.VoidedAt, after.VoidReason, after.VoidedBy = &now, reason, actor
		return writeAudit(tx, h.store, auditLine{Entity: "sale", Ref: id, Action: "void",
			Before: sale, After: after, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"ok": true, "voided": true, "restocked": restocked})
}

type saleCorrectionInput struct {
	Qty     *int    `json:"qty"`
	Trainee *string `json:"trainee"`
	Reason  string  `json:"reason"` // required when qty changes
}

// correctSale fixes a logged sale's quantity and/or buyer, adjusting stock by
// the delta. Reducing qty returns units to stock; increasing it takes more off
// the shelf (guarded so it can't drive stock negative).
func (h *inventoryHandler) correctSale(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in saleCorrectionInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	sale, err := findSale(ctx, h.store, id)
	if err != nil {
		return err
	}
	if sale.VoidedAt != nil {
		return apiErr(http.StatusConflict, CodeVoidedLocked, "sale is voided and can no longer be corrected")
	}
	reason := strings.TrimSpace(in.Reason)
	if in.Qty != nil {
		if *in.Qty <= 0 {
			return badField("qty", "quantity must be at least 1")
		}
		if *in.Qty < sale.ReturnedQty {
			return apiErr(http.StatusBadRequest, CodeReturnExceeds,
				fmt.Sprintf("%d already came back as returns, so the sale can't be for fewer", sale.ReturnedQty)).withField("qty")
		}
		if *in.Qty != sale.Qty {
			if reason, err = requireReason(in.Reason, "change a sale's quantity"); err != nil {
				return err
			}
		}
	}
	var buyer *models.Trainee
	if in.Trainee != nil && *in.Trainee != "" {
		t, err := loadTrainee(ctx, h.store, *in.Trainee, "trainee")
		if err != nil {
			return err
		}
		buyer = &t
	}
	actor := actorOf(c)
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		live, err := findSale(tx, h.store, id)
		if err != nil {
			return err
		}
		if live.VoidedAt != nil {
			return apiErr(http.StatusConflict, CodeVoidedLocked, "sale is voided and can no longer be corrected")
		}
		set := bson.M{}
		if in.Qty != nil && *in.Qty != live.Qty {
			delta := *in.Qty - live.Qty // >0 => take more off the shelf
			var it models.InventoryItem
			if delta > 0 {
				if it, err = takeStock(tx, h.store, live.Item, delta, false); err != nil {
					return err
				}
			} else {
				err = h.store.Coll(models.CollInventory).FindOneAndUpdate(tx, bson.M{"_id": live.Item},
					bson.M{"$inc": bson.M{"stock": -delta}, "$set": bson.M{"updatedAt": time.Now()}},
					options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&it)
				if errors.Is(err, mongo.ErrNoDocuments) {
					return apiErr(http.StatusConflict, CodeItemNotFound, "the item was removed from the shop, so its stock can't be corrected")
				}
				if err != nil {
					return err
				}
			}
			if err := recordMovement(tx, h.store, models.StockMovement{
				Item: live.Item, ItemName: live.ItemName, Delta: -delta, Kind: models.MoveSaleEdit, Reason: reason,
				RefType: "sale", Ref: &live.ID, StockAfter: it.Stock, Actor: actor,
			}); err != nil {
				return err
			}
			set["qty"] = *in.Qty
			set["total"] = models.Amount(models.Cents(live.UnitPrice) * int64(*in.Qty))
		}
		if in.Trainee != nil {
			if buyer == nil {
				set["trainee"] = nil
				set["traineeName"] = ""
			} else {
				set["trainee"] = buyer.ID
				set["traineeName"] = buyer.Name
			}
		}
		if len(set) == 0 {
			return nil
		}
		if _, err := h.store.Coll(models.CollSales).UpdateOne(tx, bson.M{"_id": id, "voidedAt": nil}, bson.M{"$set": set}); err != nil {
			return err
		}
		after, err := findSale(tx, h.store, id)
		if err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "sale", Ref: id, Action: "update",
			Before: live, After: after, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	out, err := findSale(ctx, h.store, id)
	if err != nil {
		return err
	}
	return c.JSON(out)
}
