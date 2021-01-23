package sessions_mongo

import (
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type session struct {
	ID           primitive.ObjectID `bson:"_id"`
	Data         string             `bson:"data"`
	LastModified time.Time          `bson:"last_modified"`
}

func (s session) ObjectID() primitive.ObjectID {
	return s.ID
}

func sessionFromGorillaSession(sess *sessions.Session, codecs ...securecookie.Codec) (session, error) {
	oid, err := primitive.ObjectIDFromHex(sess.ID)
	if err != nil {
		return session{}, err
	}

	encodedValues, err := securecookie.EncodeMulti(sess.Name(), sess.Values, codecs...)
	if err != nil {
		return session{}, err
	}

	return session{
		ID:           oid,
		Data:         encodedValues,
		LastModified: currentTime(),
	}, nil
}

func currentTime() time.Time {
	return time.Now().UTC()
}
