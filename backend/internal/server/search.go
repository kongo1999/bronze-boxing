package server

import (
	"context"
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/fuzzy"
)

type searchGroup struct {
	Kind    string      `json:"kind"`
	Total   int         `json:"total"`
	HasMore bool        `json:"hasMore"`
	Items   []searchHit `json:"items"`
}

// GET /search?q=&kinds=trainee,payment&limit=5&offset=0 — typed groups,
// best match first, each with its total so the client can offer "show
// all". didYouMean is set when nothing matched without a typo.
func registerSearch(r fiber.Router, store *db.Store) {
	r.Get("/search", func(c *fiber.Ctx) error {
		ctx, cancel := reqCtx()
		defer cancel()
		q := fuzzy.NewQuery(c.Query("q"))
		out := fiber.Map{"query": q.Norm, "groups": []searchGroup{}}
		if q.Empty() {
			return c.JSON(out)
		}
		kinds := searchKinds
		if k := strings.TrimSpace(c.Query("kinds")); k != "" {
			kinds = nil
			for _, part := range strings.Split(k, ",") {
				if err := oneOf("kinds", part, searchKinds...); err != nil {
					return err
				}
				kinds = append(kinds, part)
			}
		}
		limit := min(max(atoiDefault(c.Query("limit"), 5), 1), 100)
		offset := max(atoiDefault(c.Query("offset"), 0), 0)
		idx := searchIndexFor(store)
		found, err := idx.search(ctx, q, kinds...)
		if err != nil {
			return err
		}
		groups := []searchGroup{}
		for _, k := range kinds {
			hits := found[k]
			if len(hits) == 0 {
				continue
			}
			end := min(offset+limit, len(hits))
			page := []searchHit{}
			if offset < len(hits) {
				page = hits[offset:end]
			}
			groups = append(groups, searchGroup{Kind: k, Total: len(hits), HasMore: end < len(hits), Items: page})
		}
		out["groups"] = groups
		if dym := idx.correction(q, found); dym != "" && dym != q.Norm {
			out["didYouMean"] = dym
		}
		return c.JSON(out)
	})
}

// searchQuery reads ?q= for a list endpoint.
func searchQuery(c *fiber.Ctx) (fuzzy.Query, bool) {
	q := fuzzy.NewQuery(c.Query("q"))
	return q, !q.Empty()
}

// rankedFind answers a list endpoint's search: the kind's matches from the
// search index, narrowed by the list's own filters, in rank order — then
// paged like pagedFind (a page with ?limit=, else the plain array).
func rankedFind[T any](c *fiber.Ctx, ctx context.Context, store *db.Store, coll, kind string, filter bson.M,
	q fuzzy.Query, idOf func(T) primitive.ObjectID) error {
	ids, err := searchIndexFor(store).rankedIDs(ctx, q, kind)
	if err != nil {
		return err
	}
	pos := make(map[primitive.ObjectID]int, len(ids))
	for i, id := range ids {
		pos[id] = i
	}
	f := bson.M{}
	for k, v := range filter {
		f[k] = v
	}
	f["_id"] = bson.M{"$in": ids}
	cur, err := store.Coll(coll).Find(ctx, f)
	if err != nil {
		return err
	}
	docs := []T{}
	if err := cur.All(ctx, &docs); err != nil {
		return err
	}
	sort.SliceStable(docs, func(i, j int) bool { return pos[idOf(docs[i])] < pos[idOf(docs[j])] })
	if c.Query("limit") == "" {
		return c.JSON(docs)
	}
	limit := min(max(atoiDefault(c.Query("limit"), 20), 1), 200)
	offset := max(atoiDefault(c.Query("offset"), 0), 0)
	end := min(offset+limit, len(docs))
	items := []T{}
	if offset < len(docs) {
		items = docs[offset:end]
	}
	return c.JSON(page[T]{Items: items, Total: int64(len(docs)), HasMore: end < len(docs), Offset: offset, Limit: limit})
}
