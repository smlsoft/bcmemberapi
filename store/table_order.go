package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TableOrderSession stores a table ordering session created by kiosk
type TableOrderSession struct {
	SessionID           string    `bson:"session_id" json:"session_id"`
	OrderID             string    `bson:"order_id" json:"order_id"`
	ShopID              string    `bson:"shop_id" json:"shop_id"`
	BranchID            string    `bson:"branch_id" json:"branch_id"`
	TableNumber         string    `bson:"table_number" json:"table_number"`
	BuffetCode          string    `bson:"buffet_code" json:"buffet_code"`
	Mode                string    `bson:"mode" json:"mode"` // "alacarte" | "buffet"
	PriceIndex          int       `bson:"price_index" json:"price_index"`
	CategoryGroupNumber int       `bson:"category_group_number" json:"category_group_number"`
	DedeposToken        string    `bson:"dedepos_token" json:"dedepos_token"`
	ManCount            int       `bson:"man_count" json:"man_count"`
	WomanCount          int       `bson:"woman_count" json:"woman_count"`
	ChildCount          int       `bson:"child_count" json:"child_count"`
	Status              string    `bson:"status" json:"status"` // "open" | "ordering" | "closed"
	LineUserID          string    `bson:"line_user_id,omitempty" json:"line_user_id,omitempty"`
	DisplayName         string    `bson:"display_name,omitempty" json:"display_name,omitempty"`
	PictureURL          string    `bson:"picture_url,omitempty" json:"picture_url,omitempty"`
	CreatedAt           time.Time `bson:"created_at" json:"created_at"`
	ExpiresAt           time.Time `bson:"expires_at" json:"expires_at"` // 8 hours TTL
}

func tableOrderColl(client *mongo.Client) *mongo.Collection {
	return client.Database("bcmember").Collection("table_order_sessions")
}

// EnsureTableOrderIndexes creates indexes for table_order_sessions collection
func EnsureTableOrderIndexes(ctx context.Context, client *mongo.Client) {
	coll := tableOrderColl(client)

	// Unique index on session_id
	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "session_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("idx_table_session_id_unique"),
	})
	if err != nil {
		fmt.Printf("INFO: ensure index table session_id: %v\n", err)
	}

	// TTL index on expires_at
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "expires_at", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(0).SetName("idx_table_expires_at_ttl"),
	})
	if err != nil {
		fmt.Printf("INFO: ensure TTL index table expires_at: %v\n", err)
	}

	// Compound index on shop_id + status for polling queries
	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "shop_id", Value: 1},
			{Key: "status", Value: 1},
		},
		Options: options.Index().SetName("idx_table_shop_status"),
	})
	if err != nil {
		fmt.Printf("INFO: ensure index table shop_id+status: %v\n", err)
	}
}

// CreateTableOrderSession inserts a new table ordering session
func (s *Store) CreateTableOrderSession(ctx context.Context, sess *TableOrderSession) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	sessionBytes := make([]byte, 20)
	for i := range sessionBytes {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		sessionBytes[i] = charset[n.Int64()]
	}
	sess.SessionID = string(sessionBytes)
	sess.Status = "open"
	sess.CreatedAt = time.Now()
	sess.ExpiresAt = time.Now().Add(8 * time.Hour)

	coll := tableOrderColl(s.client)
	_, err := coll.InsertOne(ctx, sess)
	if err != nil {
		return "", err
	}
	return sess.SessionID, nil
}

// GetTableOrderSession retrieves a session by session_id
func (s *Store) GetTableOrderSession(ctx context.Context, sessionID string) (*TableOrderSession, error) {
	coll := tableOrderColl(s.client)
	var sess TableOrderSession
	err := coll.FindOne(ctx, bson.M{"session_id": sessionID}).Decode(&sess)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

// UpdateTableOrderSessionUser sets line_user_id, display_name, picture_url and status on a session
func (s *Store) UpdateTableOrderSessionUser(ctx context.Context, sessionID, lineUserID, displayName, pictureURL, status string) error {
	coll := tableOrderColl(s.client)
	filter := bson.M{
		"session_id": sessionID,
		"expires_at": bson.M{"$gt": time.Now()},
	}
	update := bson.M{
		"$set": bson.M{
			"line_user_id": lineUserID,
			"display_name": displayName,
			"picture_url":  pictureURL,
			"status":       status,
		},
	}
	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.ModifiedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

// CloseTableOrderSession sets status to "closed"
func (s *Store) CloseTableOrderSession(ctx context.Context, sessionID string) error {
	coll := tableOrderColl(s.client)
	filter := bson.M{"session_id": sessionID}
	update := bson.M{"$set": bson.M{"status": "closed"}}
	res, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.ModifiedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}
