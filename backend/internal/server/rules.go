package server

import (
	"context"
	"errors"
	"net/http"
	"strconv"
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

type rulesHandler struct{ store *db.Store }

func registerRules(r fiber.Router, store *db.Store) {
	h := &rulesHandler{store}
	r.Get("/studio-rules", h.get)
	r.Put("/studio-rules", h.publish)
	r.Get("/trainees/:id/rules-acknowledgements", h.acknowledgements)
	r.Post("/trainees/:id/rules-acknowledgements", h.acknowledge)
}

func (h *rulesHandler) latest(ctx context.Context) (models.StudioRules, error) {
	var rules models.StudioRules
	err := h.store.Coll(models.CollRules).FindOne(ctx, bson.M{}, options.FindOne().SetSort(bson.D{{Key: "version", Value: -1}})).Decode(&rules)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return models.StudioRules{}, nil
	}
	return rules, err
}

func (h *rulesHandler) get(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	if raw := c.Query("version"); raw != "" {
		version, err := strconv.Atoi(raw)
		if err != nil || version < 1 {
			return badField("version", "choose a published version")
		}
		var rules models.StudioRules
		if err := h.store.Coll(models.CollRules).FindOne(ctx, bson.M{"version": version}).Decode(&rules); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return notFound("rules version")
			}
			return err
		}
		return c.JSON(rules)
	}
	rules, err := h.latest(ctx)
	if err != nil {
		return err
	}
	return c.JSON(rules)
}

func (h *rulesHandler) publish(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	var in struct {
		GroupPolicy     string `json:"groupPolicy"`
		PrivatePolicy   string `json:"privatePolicy"`
		ClientAgreement string `json:"clientAgreement"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	in.GroupPolicy = strings.TrimSpace(in.GroupPolicy)
	in.PrivatePolicy = strings.TrimSpace(in.PrivatePolicy)
	in.ClientAgreement = strings.TrimSpace(in.ClientAgreement)
	if in.GroupPolicy == "" && in.PrivatePolicy == "" && in.ClientAgreement == "" {
		return badField("body", "add at least one policy or agreement section")
	}
	for field, value := range map[string]string{"groupPolicy": in.GroupPolicy, "privatePolicy": in.PrivatePolicy, "clientAgreement": in.ClientAgreement} {
		if len([]rune(value)) > 12000 {
			return badField(field, "keep each section under 12,000 characters")
		}
	}
	previous, err := h.latest(ctx)
	if err != nil {
		return err
	}
	if previous.Version > 0 && previous.GroupPolicy == in.GroupPolicy && previous.PrivatePolicy == in.PrivatePolicy && previous.ClientAgreement == in.ClientAgreement {
		return c.JSON(previous)
	}
	rules := models.StudioRules{
		ID: primitive.NewObjectID(), Version: previous.Version + 1,
		GroupPolicy: in.GroupPolicy, PrivatePolicy: in.PrivatePolicy, ClientAgreement: in.ClientAgreement,
		UpdatedAt: time.Now(), UpdatedBy: actorOf(c),
	}
	if _, err := h.store.Coll(models.CollRules).InsertOne(ctx, rules); err != nil {
		if isDup(err) {
			return apiErr(http.StatusConflict, CodeConflict, "rules changed while you were editing; reload and review the latest version")
		}
		return err
	}
	return c.JSON(rules)
}

func (h *rulesHandler) acknowledgements(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := loadTrainee(ctx, h.store, id.Hex(), "id"); err != nil {
		return err
	}
	rows, err := allDocs[models.RulesAcknowledgement](ctx, h.store.Coll(models.CollRulesAcks), bson.M{"trainee": id}, bson.D{{Key: "version", Value: -1}})
	if err != nil {
		return err
	}
	return c.JSON(rows)
}

func (h *rulesHandler) acknowledge(c *fiber.Ctx) error {
	ctx, cancel := reqCtx()
	defer cancel()
	id, err := objID(c)
	if err != nil {
		return err
	}
	if _, err := loadTrainee(ctx, h.store, id.Hex(), "id"); err != nil {
		return err
	}
	var in struct {
		Version int    `json:"version"`
		Method  string `json:"method"`
		Note    string `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return badField("body", "invalid body")
	}
	rules, err := h.latest(ctx)
	if err != nil {
		return err
	}
	if rules.Version == 0 || in.Version != rules.Version {
		return badField("version", "review and record the current published rules")
	}
	if err := oneOf("method", in.Method, "signed_paper", "confirmed_message", "verbal"); err != nil {
		return err
	}
	if len([]rune(in.Note)) > 500 {
		return badField("note", "keep the note under 500 characters")
	}
	ack := models.RulesAcknowledgement{ID: primitive.NewObjectID(), Trainee: id, Version: in.Version,
		Method: in.Method, Note: strings.TrimSpace(in.Note), AcknowledgedAt: time.Now(), RecordedBy: actorOf(c)}
	if _, err := h.store.Coll(models.CollRulesAcks).InsertOne(ctx, ack); err != nil {
		if isDup(err) {
			return apiErr(http.StatusConflict, CodeConflict, "this member's acknowledgement of the current rules is already recorded")
		}
		return err
	}
	return c.Status(http.StatusCreated).JSON(ack)
}
