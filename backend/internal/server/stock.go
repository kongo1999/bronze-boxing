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

	"bronzeboxing/internal/models"
)

// Stock changes other than sales — deliveries, write-offs, counted
// corrections — and the life of a sale after it happened: partial returns
// and a printable receipt. Every stock change writes a ledger line and an
// audit line in the same transaction as the change itself.

const maxAdjustQty = 100000

type adjustInput struct {
	Kind     string   `json:"kind"`     // restock | damage | correction
	Qty      int      `json:"qty"`      // restock / damage: units, > 0
	Count    *int     `json:"count"`    // correction: what's actually on the shelf
	Expected *int     `json:"expected"` // correction: the stock the user was looking at
	UnitCost *float64 `json:"unitCost"` // restock: optionally the new cost price
	Reason   string   `json:"reason"`   // required for damage and correction
}

// adjust records a delivery, a write-off or a counted correction.
func (h *inventoryHandler) adjust(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in adjustInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if err := oneOf("kind", in.Kind, models.MoveRestock, models.MoveDamage, models.MoveCorrection); err != nil {
		return err
	}
	reason := strings.TrimSpace(in.Reason)
	switch in.Kind {
	case models.MoveRestock, models.MoveDamage:
		if in.Qty <= 0 || in.Qty > maxAdjustQty {
			return badField("qty", fmt.Sprintf("quantity must be between 1 and %d", maxAdjustQty))
		}
		if in.Kind == models.MoveDamage {
			if reason, err = requireReason(in.Reason, "write off stock"); err != nil {
				return err
			}
		}
	case models.MoveCorrection:
		if in.Count == nil || *in.Count < 0 || *in.Count > maxAdjustQty {
			return badField("count", "enter the number actually on the shelf")
		}
		if reason, err = requireReason(in.Reason, "correct the stock count"); err != nil {
			return err
		}
	}
	var unitCost *float64
	if in.UnitCost != nil && in.Kind == models.MoveRestock {
		v, err := validMoney("unitCost", *in.UnitCost, true)
		if err != nil {
			return err
		}
		unitCost = &v
	}
	actor := actorOf(c)
	var item models.InventoryItem
	var move models.StockMovement
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		live, err := findItem(tx, h.store, id)
		if err != nil {
			return err
		}
		now := time.Now()
		coll := h.store.Coll(models.CollInventory)
		after := options.FindOneAndUpdate().SetReturnDocument(options.After)
		delta := 0
		switch in.Kind {
		case models.MoveRestock:
			set := bson.M{"updatedAt": now}
			if unitCost != nil {
				set["costPrice"] = *unitCost
			}
			if err := coll.FindOneAndUpdate(tx, bson.M{"_id": id},
				bson.M{"$inc": bson.M{"stock": in.Qty}, "$set": set}, after).Decode(&item); err != nil {
				return err
			}
			delta = in.Qty
		case models.MoveDamage:
			if item, err = takeStock(tx, h.store, id, in.Qty, false); err != nil {
				return err
			}
			delta = -in.Qty
		case models.MoveCorrection:
			if in.Expected != nil && *in.Expected != live.Stock {
				return apiErr(http.StatusConflict, CodeConflict,
					fmt.Sprintf("stock changed to %d while you were counting — check and try again", live.Stock)).
					withDetails(map[string]int{"stock": live.Stock})
			}
			delta = *in.Count - live.Stock
			if delta == 0 {
				return badField("count", fmt.Sprintf("%d is already the stock on record", live.Stock))
			}
			err := coll.FindOneAndUpdate(tx, bson.M{"_id": id, "stock": live.Stock},
				bson.M{"$set": bson.M{"stock": *in.Count, "updatedAt": now}}, after).Decode(&item)
			if errors.Is(err, mongo.ErrNoDocuments) {
				return apiErr(http.StatusConflict, CodeConflict, "stock changed while you were counting — reload and try again")
			}
			if err != nil {
				return err
			}
		}
		move = models.StockMovement{
			ID: primitive.NewObjectID(), Item: id, ItemName: item.Name, Delta: delta, Kind: in.Kind,
			Reason: reason, StockAfter: item.Stock, At: now, Actor: actor,
		}
		if err := recordMovement(tx, h.store, move); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "item", Ref: id, Action: in.Kind,
			Before: live, After: item, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"item": item, "movement": move})
}

// setActive archives an item (hidden from selling, history kept) or brings
// it back.
func (h *inventoryHandler) setActive(active bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := reqCtx()
		defer cancel()
		id, err := objID(c)
		if err != nil {
			return err
		}
		var in struct {
			Reason string `json:"reason"`
		}
		_ = c.BodyParser(&in)
		actor := actorOf(c)
		var item models.InventoryItem
		err = h.store.WithTx(ctx, func(tx context.Context) error {
			live, err := findItem(tx, h.store, id)
			if err != nil {
				return err
			}
			if err := h.store.Coll(models.CollInventory).FindOneAndUpdate(tx, bson.M{"_id": id},
				bson.M{"$set": bson.M{"active": active, "updatedAt": time.Now()}},
				options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&item); err != nil {
				return err
			}
			action := map[bool]string{true: "unarchive", false: "archive"}[active]
			return writeAudit(tx, h.store, auditLine{Entity: "item", Ref: id, Action: action,
				Before: live, After: item, Actor: actor, Reason: strings.TrimSpace(in.Reason)})
		})
		if err != nil {
			return err
		}
		return c.JSON(item)
	}
}

type returnInput struct {
	Qty    int      `json:"qty"`
	Amount *float64 `json:"amount"` // refund; defaults to qty × the sale's unit price
	Reason string   `json:"reason"`
	Day    string   `json:"day"` // studio day the refund was paid; default today
}

// returnSale takes back some of a sale's units: they go back on the shelf,
// the refund is recorded as money out on its own cash day, and the sale
// keeps a running total of what came back. The sale itself is never
// rewritten; a full void remains the tool for a sale that shouldn't exist.
func (h *inventoryHandler) returnSale(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	var in returnInput
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	if in.Qty <= 0 {
		return badField("qty", "return at least 1 unit")
	}
	reason, err := requireReason(in.Reason, "record a return")
	if err != nil {
		return err
	}
	date, err := resolveRecordDate(nil, in.Day)
	if err != nil {
		return err
	}
	if models.DateKey(date) > models.DateKey(time.Now()) {
		return badField("day", "a refund can't be dated in the future")
	}
	var amount *int64
	if in.Amount != nil {
		v, err := validMoney("amount", *in.Amount, true)
		if err != nil {
			return err
		}
		cents := models.Cents(v)
		amount = &cents
	}
	actor := actorOf(c)
	var ret models.SaleReturn
	var out models.Sale
	restocked := 0
	err = h.store.WithTx(ctx, func(tx context.Context) error {
		restocked = 0
		sale, err := findSale(tx, h.store, id)
		if err != nil {
			return err
		}
		if sale.VoidedAt != nil {
			return apiErr(http.StatusConflict, CodeVoidedLocked, "this sale is voided — nothing left to return")
		}
		left := sale.Qty - sale.ReturnedQty
		if in.Qty > left {
			return apiErr(http.StatusBadRequest, CodeReturnExceeds,
				fmt.Sprintf("only %d of this sale can still come back", left)).
				withField("qty").withDetails(map[string]int{"returnable": left})
		}
		refundable := models.Cents(sale.Total) - models.Cents(sale.ReturnedTotal)
		cents := models.Cents(sale.UnitPrice) * int64(in.Qty)
		if amount != nil {
			cents = *amount
		}
		if cents > refundable {
			return apiErr(http.StatusBadRequest, CodeReturnExceeds,
				fmt.Sprintf("the refund can be at most %s — what's left of this sale", fmtAmt(models.Amount(refundable)))).
				withField("amount").withDetails(map[string]float64{"refundable": models.Amount(refundable)})
		}
		now := time.Now()
		// Guarded on the returned count we read, so two returns racing on the
		// same sale can't both pass the check above.
		res, err := h.store.Coll(models.CollSales).UpdateOne(tx, bson.M{
			"_id": id, "voidedAt": nil,
			"$expr": bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$returnedQty", 0}}, sale.ReturnedQty}},
		}, bson.M{"$set": bson.M{
			"returnedQty":   sale.ReturnedQty + in.Qty,
			"returnedTotal": models.Amount(models.Cents(sale.ReturnedTotal) + cents),
		}})
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return apiErr(http.StatusConflict, CodeConflict, "this sale changed while you were recording the return — reload and try again")
		}
		ret = models.SaleReturn{
			ID: primitive.NewObjectID(), Sale: sale.ID, Item: sale.Item, ItemName: sale.ItemName,
			Qty: in.Qty, Amount: models.Amount(cents), Date: date, Reason: reason, CreatedAt: now, Actor: actor,
		}
		var it models.InventoryItem
		err = h.store.Coll(models.CollInventory).FindOneAndUpdate(tx, bson.M{"_id": sale.Item},
			bson.M{"$inc": bson.M{"stock": in.Qty}, "$set": bson.M{"updatedAt": now}},
			options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&it)
		switch {
		case errors.Is(err, mongo.ErrNoDocuments):
			// The item is gone from the shop: the refund stands, nothing to restock.
		case err != nil:
			return err
		default:
			restocked = in.Qty
			if err := recordMovement(tx, h.store, models.StockMovement{
				Item: it.ID, ItemName: it.Name, Delta: in.Qty, Kind: models.MoveReturn, Reason: reason,
				RefType: "return", Ref: &ret.ID, StockAfter: it.Stock, At: now, Actor: actor,
			}); err != nil {
				return err
			}
		}
		if _, err := h.store.Coll(models.CollReturns).InsertOne(tx, ret); err != nil {
			return err
		}
		if out, err = findSale(tx, h.store, id); err != nil {
			return err
		}
		return writeAudit(tx, h.store, auditLine{Entity: "sale", Ref: id, Action: "return",
			Before: sale, After: out, Actor: actor, Reason: reason})
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"return": ret, "sale": out, "restocked": restocked})
}

func (h *inventoryHandler) saleReturns(ctx context.Context, saleID primitive.ObjectID) ([]models.SaleReturn, error) {
	cur, err := h.store.Coll(models.CollReturns).Find(ctx, bson.M{"sale": saleID},
		options.Find().SetSort(bson.D{{Key: "date", Value: 1}, {Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	out := []models.SaleReturn{}
	return out, cur.All(ctx, &out)
}

func (h *inventoryHandler) listReturns(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	out, err := h.saleReturns(ctx, id)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// saleReceipt is the printable/shareable record of a shop sale, with any
// returns against it and what the buyer paid net.
func (h *inventoryHandler) saleReceipt(c *fiber.Ctx) error {
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
	returns, err := h.saleReturns(ctx, id)
	if err != nil {
		return err
	}
	hex := sale.ID.Hex()
	return c.JSON(fiber.Map{
		"studioInfo": studio,
		"number":     "S-" + strings.ToUpper(hex[len(hex)-8:]),
		"sale":       sale,
		"returns":    returns,
		"net":        models.Amount(models.Cents(sale.Total) - models.Cents(sale.ReturnedTotal)),
		"issued":     time.Now().UTC(),
		"void":       sale.VoidedAt != nil,
		"method":     defaultStr(sale.Method, models.MethodCash),
		"cashDay":    models.DateKey(sale.Date),
		"timezone":   time.Local.String(),
	})
}
