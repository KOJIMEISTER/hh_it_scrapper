package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStore struct {
	Collection                *mongo.Collection
	existingVacancyIDs        map[string]struct{}
	existingDescriptionHashes *sync.Map
}

func NewMongoStore(uri, dbName, collectionName string) (*MongoStore, error) {
	clientOptions := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		return nil, fmt.Errorf("MongoDB connection error: %w", err)
	}

	collection := client.Database(dbName).Collection(collectionName)
	return &MongoStore{
		Collection: collection,
	}, nil
}

func (s *MongoStore) LoadExistingData() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	s.existingVacancyIDs = make(map[string]struct{})
	s.existingDescriptionHashes = &sync.Map{}

	cursor, err := s.Collection.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{
		{"id", 1},
		{"description_hash", 1},
	}))
	if err != nil {
		return fmt.Errorf("failed to fetch existing vacancies: %w", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var doc struct {
			ID              string `bson:"id"`
			DescriptionHash string `bson:"description_hash"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return fmt.Errorf("failed to decode document: %w", err)
		}
		s.existingVacancyIDs[doc.ID] = struct{}{}
		if doc.DescriptionHash != "" {
			s.existingDescriptionHashes.Store(doc.DescriptionHash, struct{}{})
		}
	}

	return cursor.Err()
}

func (s *MongoStore) VacancyExists(id string) bool {
	_, exists := s.existingVacancyIDs[id]
	return exists
}

func (s *MongoStore) DescriptionHashExists(hash string) bool {
	_, exists := s.existingDescriptionHashes.Load(hash)
	return exists
}

func (s *MongoStore) AddDescriptionHash(hash string) {
	s.existingDescriptionHashes.Store(hash, struct{}{})
}

func (s *MongoStore) UpsertVacancy(data map[string]interface{}) error {
	filter := bson.M{"id": data["id"]}
	update := bson.M{"$set": data}
	_, err := s.Collection.UpdateOne(context.TODO(), filter, update, options.Update().SetUpsert(true))
	return err
}
