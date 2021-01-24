package sessions_mongo

import (
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

func makeTTLIndexModel(ttl time.Duration) mongo.IndexModel {
	idxOpts := options.Index().SetExpireAfterSeconds(int32(ttl.Seconds()))
	return mongo.IndexModel{
		Keys: bson.D{
			{
				Key:   "last_modified",
				Value: 1,
			},
		},
		Options: idxOpts,
	}
}
