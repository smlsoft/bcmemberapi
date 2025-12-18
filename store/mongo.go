package store

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type LoginCode struct {
	Code        string    `bson:"code" json:"code"`
	ShopID      string    `bson:"shop_id,omitempty" json:"shop_id,omitempty"`
	Status      string    `bson:"status" json:"status"` // "pending", "success"
	LineUserID  string    `bson:"line_user_id,omitempty" json:"line_user_id,omitempty"`
	DisplayName string    `bson:"display_name,omitempty" json:"display_name,omitempty"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	ExpiresAt   time.Time `bson:"expires_at" json:"expires_at"`
}

type ChatMessage struct {
	UserID    string    `bson:"user_id" json:"user_id"`
	Role      string    `bson:"role" json:"role"` // "user" or "assistant"
	Message   string    `bson:"message" json:"message"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

type Store struct {
	client       *mongo.Client
	coll         *mongo.Collection
	chatHistColl *mongo.Collection
}

func NewStore(uri string) (*Store, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	db := client.Database("bcmember")
	coll := db.Collection("login_codes")
	chatHistColl := db.Collection("chat_history")

	return &Store{
		client:       client,
		coll:         coll,
		chatHistColl: chatHistColl,
	}, nil
}

func (s *Store) GenerateCode(ctx context.Context, shopID string) (string, error) {
	code := ""
	for i := 0; i < 4; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(10))
		code += n.String()
	}

	loginCode := LoginCode{
		Code:      code,
		ShopID:    shopID,
		Status:    "pending",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	_, err := s.coll.InsertOne(ctx, loginCode)
	if err != nil {
		return "", err
	}
	return code, nil
}

func (s *Store) VerifyCode(ctx context.Context, code string, userID, displayName string) error {
	filter := bson.M{
		"code":       code,
		"status":     "pending",
		"expires_at": bson.M{"$gt": time.Now()},
	}
	update := bson.M{
		"$set": bson.M{
			"status":       "success",
			"line_user_id": userID,
			"display_name": displayName,
		},
	}
	res, err := s.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.ModifiedCount == 0 {
		return mongo.ErrNoDocuments
	}
	return nil
}

func (s *Store) GetCodeStatus(ctx context.Context, code string) (*LoginCode, error) {
	var result LoginCode
	err := s.coll.FindOne(ctx, bson.M{"code": code}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SaveChatMessage saves a chat message to the database
func (s *Store) SaveChatMessage(ctx context.Context, userID, role, message string) error {
	chatMsg := ChatMessage{
		UserID:    userID,
		Role:      role,
		Message:   message,
		CreatedAt: time.Now(),
	}
	_, err := s.chatHistColl.InsertOne(ctx, chatMsg)
	return err
}

// GetChatHistory retrieves chat history for a user, limited to the last N messages
func (s *Store) GetChatHistory(ctx context.Context, userID string, limit int64) ([]ChatMessage, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(limit)

	cursor, err := s.chatHistColl.Find(ctx, bson.M{"user_id": userID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []ChatMessage
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, err
	}

	// Reverse the messages so the oldest is first
	for i := 0; i < len(messages)/2; i++ {
		j := len(messages) - i - 1
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}
