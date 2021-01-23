package sessions_mongo

import (
	"context"
	"github.com/go-kit/kit/log"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/http"
	"os"
	"testing"
	"time"
)

//Test Cases:
//-Get
//--Cookie Exists
//--Cookie doesn't exist
//-Save
//--Max Age <= 0 -- delete
//----Expect nil err; expect session no longer in DB
//--Cookie saved
//----Expect nil err; expect session in DB
//-New
//--In DB
//--Not in DB

const (
	MONGO_HOST = "MONGO_HOST"

	TEST_DATABASE   = "sessions_test"
	TEST_COLLECTION = "gosess"
)

type mongoSuite struct {
	suite.Suite
	client     *mongo.Client
	collection *mongo.Collection
}

func (ms *mongoSuite) SetupSuite() {
	mongoHost, exists := os.LookupEnv(MONGO_HOST)
	if !exists {
		mongoHost = "localhost:27017"
	}
	connectionOpts := options.Client().SetHosts([]string{mongoHost}).SetConnectTimeout(5 * time.Second)

	var err error
	ms.client, err = mongo.NewClient(connectionOpts)
	require.Nil(ms.T(), err)
	require.Nil(ms.T(), ms.client.Connect(context.Background()))

	ms.collection = ms.client.Database(TEST_DATABASE).Collection(TEST_COLLECTION)
}

func (ms *mongoSuite) TearDownSuite() {
	ms.collection.Drop(context.Background())
	ms.client.Disconnect(context.Background())
}

func TestNewMongoDBStore(t *testing.T) {
	cs := new(CreationSuite)
	suite.Run(t, cs)
}

type CreationSuite struct {
	mongoSuite
}

func (cs *CreationSuite) TestNewMongoDBStore() {
	type tc struct {
		description    string
		storeOptions   Options
		sessionOptions *sessions.Options
		codecs         []securecookie.Codec
		expectedErr    error
	}

	tcs := []tc{
		{
			description: "fully populated",
			storeOptions: Options{
				TTLOptions: TTLOptions{
					TTL:            500 * time.Second,
					EnsureTTLIndex: true,
				},
			},
			sessionOptions: &sessions.Options{
				Path:   "testPath",
				MaxAge: 245,
			},
		},
		{
			description: "invalid TTLOptions",
			storeOptions: Options{
				TTLOptions: TTLOptions{
					TTL:            0 * time.Second,
					EnsureTTLIndex: false,
				},
			},
			sessionOptions: &sessions.Options{
				Path:   "testPath",
				MaxAge: 209,
			},
			expectedErr: NewInvalidTTLErr(0 * time.Second),
		},
	}

	for _, testCase := range tcs {
		cs.T().Run(testCase.description, func(t *testing.T) {
			err := cs.collection.Drop(context.Background())
			require.Nil(t, err)
			store, err := NewMongoDBStore(cs.collection, testCase.storeOptions, testCase.sessionOptions, nil, testCase.codecs...)
			assert.Equal(t, testCase.expectedErr, err)

			if err == nil {
				assert.Equal(t, cs.collection, store.collection)
				assert.Equal(t, testCase.storeOptions, store.storeOptions)
				assert.Equal(t, testCase.sessionOptions, store.defaultOptions)
				assert.Equal(t, log.NewNopLogger(), store.logger)

				if testCase.storeOptions.TTLOptions.EnsureTTLIndex {
					cs.collection.Indexes().List(context.Background())
				}
			}
		})
	}
}

func TestMongoDBStore_Save(t *testing.T) {
	ss := new(SaveSuite)
	suite.Run(t, ss)
}

type SaveSuite struct {
	mongoSuite
	store *MongoDBStore
}

func (ss *SaveSuite) SetupSuite() {
	ss.mongoSuite.SetupSuite()
	var err error
	ss.store, err = NewMongoDBStore(
		ss.collection,
		Options{TTLOptions: TTLOptions{ TTL: 5 * time.Second, EnsureTTLIndex: false}},
		&sessions.Options{
			MaxAge: 50,
		},
		log.NewNopLogger(),
		securecookie.CodecsFromPairs([]byte("abcdefghijklmnop"))...,
	)
	require.Nil(ss.T(), err)
}

func (ss *SaveSuite) TestMongoDBStore_Save() {
	sessionKey := "session-name"
	rw := NewMockResponseWriter()
	r, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	sessionToSave, err := ss.store.New(r, sessionKey)
	assert.Nil(ss.T(), err)
	assert.True(ss.T(), sessionToSave.IsNew)
	sessionToSave.Values = map[interface{}]interface{}{
		"key": "value",
		"i":   5,
		"s":   "92",
		"f":   10.5,
	}

	err = ss.store.Save(r, rw, sessionToSave)
	assert.Nil(ss.T(), err)
	assert.False(ss.T(), sessionToSave.IsNew)
	assertSessionStoredProperlyInCookie(ss.T(), sessionKey, sessionToSave, ss.store, rw)
	assertSessionStoredProperlyInDB(ss.T(), sessionToSave, ss.store)
}

func (ss *SaveSuite) TestMongoDBStore_Save_BadID() {
	s := sessions.NewSession(ss.store, "key")
	s.ID = "abcdee"
	s.Values = map[interface{}]interface{}{
		"will not": "save",
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	rw := NewMockResponseWriter()

	err := ss.store.Save(req, rw, s)
	assert.NotNil(ss.T(), err)
	assert.Equal(ss.T(), primitive.ErrInvalidHex, err)
}

func (ss *SaveSuite) TestMongoDBStore_Save_MaxAgeIsZero() {
	r, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	sessionKey := "key"
	sess, err := ss.store.New(r, sessionKey)
	assert.Nil(ss.T(), err)
	require.Greater(ss.T(), sess.Options.MaxAge, 0)
	sess.Values = map[interface{}]interface{}{
		"will be": "saved",
	}
	rw := NewMockResponseWriter()
	err = ss.store.Save(r, rw, sess)
	assertSessionStoredProperlyInCookie(ss.T(), sessionKey, sess, ss.store, rw)

	sess.Options.MaxAge = 0
	err = ss.store.Save(r, rw, sess)

	err = ss.store.load(context.Background(), sess)
	assert.Equal(ss.T(), mongo.ErrNoDocuments, err)
}

func assertSessionStoredProperlyInCookie(
	t *testing.T,
	sessionKey string,
	session *sessions.Session,
	store *MongoDBStore,
	rw http.ResponseWriter,
) {
	encodedSession, err := securecookie.EncodeMulti(sessionKey, session.ID, store.codecs...)
	require.Nil(t, err)
	expectedCookieString := sessions.NewCookie(sessionKey, encodedSession, store.defaultOptions).String()
	cookieString := rw.Header().Get("Set-Cookie")
	assert.Equal(t, expectedCookieString, cookieString)
}

func assertSessionStoredProperlyInDB(t *testing.T, expectedSession *sessions.Session, store *MongoDBStore) {
	s := sessions.NewSession(store, expectedSession.Name())
	s.Options = store.defaultOptions
	s.ID = expectedSession.ID
	err := store.load(context.Background(), s)
	require.Nil(t, err)

	assert.Equal(t, expectedSession, s)
}
