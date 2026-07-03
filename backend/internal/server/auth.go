package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"

	"bronzeboxing/internal/db"
	"bronzeboxing/internal/models"
)

// Username/password auth with server-side sessions.
//
// Auth is ON when ADMIN_PASSWORD is configured (production); empty means open
// mode (local dev). On login we issue a random bearer token whose SHA-256 is
// stored in auth_sessions with a TTL; every request then carries
// "Authorization: Bearer <token>". Logout (or rotating the admin password)
// revokes sessions server-side.

const sessionTTL = 30 * 24 * time.Hour

// Name of the httpOnly session cookie the browser carries. The raw token is
// never exposed to JS (localStorage is XSS-readable); API/CLI clients may still
// send it as a Bearer header instead.
const sessionCookieName = "bb_session"

// dummyHash keeps login timing flat when the username doesn't exist.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

// EnsureAuth creates the auth indexes and syncs the admin account with env
// config. The .env is the source of truth for the admin credential: a changed
// ADMIN_PASSWORD re-hashes the stored one and revokes the account's sessions.
func EnsureAuth(ctx context.Context, store *db.Store, username, password string) error {
	if _, err := store.Coll(models.CollAuthSessions).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "expiresAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)},
		{Keys: bson.D{{Key: "tokenHash", Value: 1}}, Options: options.Index().SetUnique(true)},
	}); err != nil {
		return err
	}
	if _, err := store.Coll(models.CollUsers).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "username", Value: 1}}, Options: options.Index().SetUnique(true),
	}); err != nil {
		return err
	}

	if password == "" {
		return nil // open mode — nothing to seed
	}
	username = normalizeUsername(username)

	var u models.User
	err := store.Coll(models.CollUsers).FindOne(ctx, bson.M{"username": username}).Decode(&u)
	switch {
	case errors.Is(err, mongo.ErrNoDocuments):
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		_, err = store.Coll(models.CollUsers).InsertOne(ctx, models.User{
			Username:     username,
			PasswordHash: string(hash),
			Role:         models.RoleAdmin,
			CreatedAt:    time.Now(),
		})
		return err
	case err != nil:
		return err
	default:
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil {
			return nil // unchanged
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := store.Coll(models.CollUsers).UpdateOne(ctx, bson.M{"_id": u.ID},
			bson.M{"$set": bson.M{"passwordHash": string(hash)}}); err != nil {
			return err
		}
		// Password rotated → everyone holding an old session logs in again.
		_, err = store.Coll(models.CollAuthSessions).DeleteMany(ctx, bson.M{"user": u.ID})
		return err
	}
}

// registerAuth wires /api/auth/*. Login sits in front of requireSession (it's
// exempted there); logout and me run behind it.
func registerAuth(r fiber.Router, store *db.Store, enabled bool) {
	g := r.Group("/auth")
	// Brute-force damper: 10 login attempts per client IP per minute. Keyed on
	// the real client IP (see clientIP) so the whole studio isn't throttled as
	// one bucket behind the reverse proxy.
	g.Post("/login", limiter.New(limiter.Config{
		Max:          10,
		Expiration:   time.Minute,
		KeyGenerator: clientIP,
	}), loginHandler(store, enabled))
	g.Post("/logout", logoutHandler(store))
	g.Get("/me", meHandler())
}

func loginHandler(store *db.Store, enabled bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !enabled {
			return fiber.NewError(fiber.StatusBadRequest, "auth is not enabled on this server")
		}
		var in struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Remember bool   `json:"remember"`
		}
		if err := c.BodyParser(&in); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid body")
		}
		in.Username = normalizeUsername(in.Username)
		if in.Username == "" || in.Password == "" {
			return fiber.NewError(fiber.StatusBadRequest, "username and password are required")
		}

		ctx, cancel := reqCtx()
		defer cancel()
		var u models.User
		err := store.Coll(models.CollUsers).FindOne(ctx, bson.M{"username": in.Username}).Decode(&u)
		if err != nil {
			// Burn a compare anyway so unknown-user and wrong-password take
			// the same time.
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(in.Password))
			return fiber.NewError(fiber.StatusUnauthorized, "wrong username or password")
		}
		if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "wrong username or password")
		}

		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		token := hex.EncodeToString(raw)
		if _, err := store.Coll(models.CollAuthSessions).InsertOne(ctx, models.AuthSession{
			TokenHash: hashToken(token),
			User:      u.ID,
			Username:  u.Username,
			ExpiresAt: time.Now().Add(sessionTTL),
			CreatedAt: time.Now(),
		}); err != nil {
			return err
		}
		// Hand the browser an httpOnly cookie so JS never holds the token.
		// "Remember me" → a persistent cookie; otherwise a session cookie that
		// dies with the browser. Secure turns on automatically behind HTTPS.
		ck := &fiber.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			HTTPOnly: true,
			Secure:   c.Get(fiber.HeaderXForwardedProto) == "https",
			SameSite: "Lax",
		}
		if in.Remember {
			ck.Expires = time.Now().Add(sessionTTL)
		}
		c.Cookie(ck)
		return c.JSON(fiber.Map{"token": token, "user": u})
	}
}

func logoutHandler(store *db.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := reqCtx()
		defer cancel()
		if raw := sessionToken(c); raw != "" {
			_, _ = store.Coll(models.CollAuthSessions).DeleteOne(ctx, bson.M{"tokenHash": hashToken(raw)})
		}
		// Expire the cookie on the client too.
		c.Cookie(&fiber.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			HTTPOnly: true,
			Expires:  time.Now().Add(-time.Hour),
			MaxAge:   -1,
		})
		return c.JSON(fiber.Map{"ok": true})
	}
}

func meHandler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if s, ok := c.Locals("session").(models.AuthSession); ok {
			return c.JSON(fiber.Map{"username": s.Username})
		}
		// Open mode (no auth middleware): there is no session identity.
		return c.JSON(fiber.Map{"username": ""})
	}
}

// requireSession guards every /api route except health (uptime checks) and
// login (how you get a session in the first place).
func requireSession(store *db.Store) fiber.Handler {
	return func(c *fiber.Ctx) error {
		switch c.Path() {
		case "/api/health", "/api/auth/login":
			return c.Next()
		}
		raw := sessionToken(c)
		if raw == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}
		ctx, cancel := reqCtx()
		defer cancel()
		var s models.AuthSession
		if err := store.Coll(models.CollAuthSessions).FindOne(ctx, bson.M{
			"tokenHash": hashToken(raw),
			"expiresAt": bson.M{"$gt": time.Now()},
		}).Decode(&s); err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
		}
		// Sliding expiration: keep a regularly-used session alive so an active
		// admin is never abruptly signed out, while idle sessions still lapse
		// via the TTL index. Renew at most ~once/day to avoid a write per call.
		if time.Until(s.ExpiresAt) < sessionTTL-24*time.Hour {
			_, _ = store.Coll(models.CollAuthSessions).UpdateOne(ctx,
				bson.M{"_id": s.ID}, bson.M{"$set": bson.M{"expiresAt": time.Now().Add(sessionTTL)}})
		}
		c.Locals("session", s)
		return c.Next()
	}
}

func bearerToken(c *fiber.Ctx) string {
	return strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
}

// sessionToken reads the session from the httpOnly cookie (browser) and falls
// back to the Authorization: Bearer header (API/CLI clients).
func sessionToken(c *fiber.Ctx) string {
	if v := c.Cookies(sessionCookieName); v != "" {
		return v
	}
	return bearerToken(c)
}

// clientIP is the real client IP even behind the reverse proxy. Caddy appends
// the connecting client's IP as the LAST X-Forwarded-For entry, so the
// rightmost value is authoritative — a client-sent XFF prefix can't spoof it
// because Caddy is the only ingress to this service.
func clientIP(c *fiber.Ctx) string {
	if xff := c.Get(fiber.HeaderXForwardedFor); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return c.IP()
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeUsername(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}
