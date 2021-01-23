package sessions_mongo

import (
	"context"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"net/http"
	"time"
)

type MongoDBStore struct {
	collection     *mongo.Collection
	ttl            time.Duration
	codecs         []securecookie.Codec
	defaultOptions *sessions.Options
	storeOptions   Options
	logger         log.Logger
}

func NewMongoDBStore(
	collection *mongo.Collection,
	storeOptions Options,
	sessionOptions *sessions.Options,
	logger log.Logger,
	codecs ...securecookie.Codec,
) (*MongoDBStore, error) {
	if !storeOptions.EnableLogging {
		logger = log.NewNopLogger()
	}

	err := ensureConnection(context.Background(), collection)
	if err != nil {
		level.Error(logger).Log("message", "failed to create connection to mongo", "error", err)
		return nil, err
	}

	if err = storeOptions.Validate(); err != nil {
		return nil, err
	}

	if storeOptions.TTLOptions.EnsureTTLIndex {
		err = ensureTTLIndex(context.Background(), collection, storeOptions.TTLOptions.TTL)
		if err != nil {
			_ = level.Error(logger).Log("message", "failed to ensure TTL index", "error", err)
			return nil, err
		}
	}

	if sessionOptions == nil {
		sessionOptions = &sessions.Options{
			Path:   "/",
			MaxAge: int(storeOptions.TTLOptions.TTL.Seconds()),
		}
		_ = level.Debug(logger).Log("message", "nil options found, using defaults")
	}
	_ = level.Info(logger).Log("cookie options", fmt.Sprintf("%+v", sessionOptions))

	return &MongoDBStore{
		collection:     collection,
		codecs:         codecs,
		ttl:            storeOptions.TTLOptions.TTL,
		storeOptions:   storeOptions,
		defaultOptions: sessionOptions,
		logger:         logger,
	}, nil
}

func (store *MongoDBStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	return sessions.GetRegistry(r).Get(store, name)
}

func (store *MongoDBStore) Save(r *http.Request, w http.ResponseWriter, sess *sessions.Session) error {
	var err error
	if sess.Options.MaxAge <= 0 {
		return store.clearSession(r.Context(), w, sess)
	}

	if sess.ID == "" {
		sess.ID = primitive.NewObjectID().Hex()
	}

	if err = store.save(r.Context(), sess); err != nil {
		return err
	}
	sess.IsNew = false

	encodedID, err := securecookie.EncodeMulti(sess.Name(), sess.ID, store.codecs...)
	if err != nil {
		_ = level.Error(store.logger).Log(
			"message", "failed to encode session ID",
			"sessionID", sess.ID,
			"error", err,
		)
		return err
	}
	http.SetCookie(w, sessions.NewCookie(sess.Name(), encodedID, sess.Options))

	return nil
}

func (store *MongoDBStore) clearSession(ctx context.Context, w http.ResponseWriter, sess *sessions.Session) error {
	if !sess.IsNew {
		if err := store.delete(ctx, sess.ID); err != nil {
			_ = level.Info(store.logger).Log(
				"message", "failed to delete session ID",
				"sessionID", sess.ID,
				"error", err,
			)
			http.SetCookie(w, sessions.NewCookie(sess.Name(), "", sess.Options))
			return err
		}
	}

	http.SetCookie(w, sessions.NewCookie(sess.Name(), "", sess.Options))
	return nil
}

func (store *MongoDBStore) save(ctx context.Context, sess *sessions.Session) error {
	s, err := sessionFromGorillaSession(sess, store.codecs...)
	if err != nil {
		_ = level.Error(store.logger).Log(
			"message", "failed to transform session",
			"error", err,
		)
		return err
	}

	return store.saveSession(ctx, s)
}

func (store *MongoDBStore) saveSession(ctx context.Context, sess session) error {
	opts := options.Update().SetUpsert(true)
	update := updateDocFromSession(sess)
	_, err := store.collection.UpdateOne(ctx, bson.M{"_id": sess.ID}, update, opts)
	if err != nil {
		_ = level.Error(store.logger).Log(
			"message", "failed to save session in database",
			"session_id", sess.ID.String(),
			"error", err,
		)
		return err
	}

	return nil
}
func (store *MongoDBStore) delete(ctx context.Context, sessionID string) error {
	oid, err := primitive.ObjectIDFromHex(sessionID)
	if err != nil {
		return err
	}

	return store.collection.FindOneAndDelete(ctx, bson.M{"_id": oid}).Err()
}

func (store *MongoDBStore) New(r *http.Request, sessionKey string) (*sessions.Session, error) {
	sess := sessions.NewSession(store, sessionKey)
	sess.ID = primitive.NewObjectID().Hex()
	sess.Options = derefOpts(store.defaultOptions)
	sess.IsNew = true

	var cookie *http.Cookie
	var err error

	if cookie, err = r.Cookie(sessionKey); err != nil {
		return sess, nil
	}

	err = securecookie.DecodeMulti(sessionKey, cookie.Value, &sess.ID, store.codecs...)
	if err != nil {
		return sess, err
	}

	err = store.load(r.Context(), sess)
	if err != nil {
		return sess, err
	}
	sess.IsNew = false

	return sess, nil
}

func (store *MongoDBStore) load(ctx context.Context, sess *sessions.Session) error {
	oid, err := primitive.ObjectIDFromHex(sess.ID)
	if err != nil {
		_ = level.Debug(store.logger).Log(
			"message", "invalid sessionID, must be BSON ID",
			"session_id", sess.ID,
			"error", err,
		)
		return err
	}

	var s session
	if err = store.collection.FindOne(ctx, bson.M{"_id": oid}).Decode(&s); err != nil {
		_ = level.Error(store.logger).Log(
			"message", "failed to load allegedly existing session",
			"session_id", sess.ID,
			"error", err,
			)
		return err
	}

	if err = securecookie.DecodeMulti(sess.Name(), s.Data,
		&sess.Values, store.codecs...); err != nil {
		return err
	}

	return nil
}

func ensureConnection(ctx context.Context, c *mongo.Collection) error {
	return c.Database().Client().Ping(ctx, readpref.PrimaryPreferred())
}

func ensureTTLIndex(ctx context.Context, collection *mongo.Collection, ttl time.Duration) error {
	idxOpts := options.CreateIndexes().SetMaxTime(15 * time.Second)
	_, err := collection.Indexes().CreateOne(ctx, makeTTLIndexModel(ttl), idxOpts)
	if err != nil {
		return err
	}

	return nil
}

func derefOpts(opts *sessions.Options) *sessions.Options {
	o := *opts
	return &o
}

func updateDocFromSession(sess session) bson.M {
	return bson.M{
		"$set": bson.M{
			"data":          sess.Data,
			"last_modified": time.Now().UTC(),
		},
	}
}
