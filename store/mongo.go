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
	PictureURL  string    `bson:"picture_url,omitempty" json:"picture_url,omitempty"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	ExpiresAt   time.Time `bson:"expires_at" json:"expires_at"`
}

type ChatMessage struct {
	UserID    string    `bson:"user_id" json:"user_id"`
	Role      string    `bson:"role" json:"role"` // "user" or "assistant"
	Message   string    `bson:"message" json:"message"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// PointTransaction represents a point transaction record
type PointTransaction struct {
	LineUID   string    `bson:"line_uid" json:"line_uid"`
	ShopID    string    `bson:"shop_id" json:"shop_id"`
	ShopName  string    `bson:"shop_name" json:"shop_name"`
	DocNo     string    `bson:"doc_no" json:"doc_no"`
	GetPoint  float64   `bson:"get_point" json:"get_point"`
	UsePoint  float64   `bson:"use_point" json:"use_point"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// Member represents a LINE member with point balance per shop
type Member struct {
	LineUID      string    `bson:"line_uid" json:"line_uid"`
	ShopID       string    `bson:"shop_id" json:"shop_id"`
	ShopName     string    `bson:"shop_name" json:"shop_name"`
	DisplayName  string    `bson:"display_name" json:"display_name"`
	PictureURL   string    `bson:"picture_url" json:"picture_url"`
	PointBalance float64   `bson:"point_balance" json:"point_balance"`
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

type Store struct {
	client         *mongo.Client
	coll           *mongo.Collection
	chatHistColl   *mongo.Collection
	pointTransColl *mongo.Collection
	membersColl    *mongo.Collection
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
	pointTransColl := db.Collection("point_transactions")
	membersColl := db.Collection("members")

	return &Store{
		client:         client,
		coll:           coll,
		chatHistColl:   chatHistColl,
		pointTransColl: pointTransColl,
		membersColl:    membersColl,
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

func (s *Store) VerifyCode(ctx context.Context, code string, userID, displayName, pictureURL string) error {
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
			"picture_url":  pictureURL,
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

// SavePointTransaction saves a point transaction to the database
func (s *Store) SavePointTransaction(ctx context.Context, lineUID, shopID, shopName, docNo string, getPoint, usePoint float64) error {
	trans := PointTransaction{
		LineUID:   lineUID,
		ShopID:    shopID,
		ShopName:  shopName,
		DocNo:     docNo,
		GetPoint:  getPoint,
		UsePoint:  usePoint,
		CreatedAt: time.Now(),
	}
	_, err := s.pointTransColl.InsertOne(ctx, trans)
	return err
}

// UpsertMember creates or updates a member record
func (s *Store) UpsertMember(ctx context.Context, lineUID, shopID, shopName, displayName, pictureURL string) error {
	filter := bson.M{
		"line_uid": lineUID,
		"shop_id":  shopID,
	}
	update := bson.M{
		"$set": bson.M{
			"shop_name":    shopName,
			"display_name": displayName,
			"picture_url":  pictureURL,
			"updated_at":   time.Now(),
		},
		"$setOnInsert": bson.M{
			"line_uid":      lineUID,
			"shop_id":       shopID,
			"point_balance": 0,
			"created_at":    time.Now(),
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := s.membersColl.UpdateOne(ctx, filter, update, opts)
	return err
}

// UpdateMemberPoints updates a member's point balance
func (s *Store) UpdateMemberPoints(ctx context.Context, lineUID, shopID string, pointChange float64) (float64, error) {
	filter := bson.M{
		"line_uid": lineUID,
		"shop_id":  shopID,
	}
	update := bson.M{
		"$inc": bson.M{"point_balance": pointChange},
		"$set": bson.M{"updated_at": time.Now()},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var member Member
	err := s.membersColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&member)
	if err != nil {
		return 0, err
	}
	return member.PointBalance, nil
}

// GetMembersByLineUID retrieves all members for a LINE user
func (s *Store) GetMembersByLineUID(ctx context.Context, lineUID string) ([]Member, error) {
	cursor, err := s.membersColl.Find(ctx, bson.M{"line_uid": lineUID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var members []Member
	if err := cursor.All(ctx, &members); err != nil {
		return nil, err
	}
	return members, nil
}

// GetMemberByLineUIDAndShopID retrieves a member by LINE UID and Shop ID
func (s *Store) GetMemberByLineUIDAndShopID(ctx context.Context, lineUID, shopID string) (*Member, error) {
	filter := bson.M{
		"line_uid": lineUID,
		"shop_id":  shopID,
	}
	var member Member
	err := s.membersColl.FindOne(ctx, filter).Decode(&member)
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// RecalculatePoints recalculates point balance from transactions
func (s *Store) RecalculatePoints(ctx context.Context, lineUID, shopID string) (int, error) {
	// Build filter for transactions
	filter := bson.M{}
	if lineUID != "" {
		filter["line_uid"] = lineUID
	}
	if shopID != "" {
		filter["shop_id"] = shopID
	}

	// Aggregate to calculate total points per member
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"line_uid": "$line_uid",
				"shop_id":  "$shop_id",
			},
			"total_get": bson.M{"$sum": "$get_point"},
			"total_use": bson.M{"$sum": "$use_point"},
			"shop_name": bson.M{"$last": "$shop_name"},
		}}},
	}

	cursor, err := s.pointTransColl.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	count := 0
	for cursor.Next(ctx) {
		var result struct {
			ID struct {
				LineUID string `bson:"line_uid"`
				ShopID  string `bson:"shop_id"`
			} `bson:"_id"`
			TotalGet float64 `bson:"total_get"`
			TotalUse float64 `bson:"total_use"`
			ShopName string  `bson:"shop_name"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}

		balance := result.TotalGet - result.TotalUse

		// Update member's point balance
		memberFilter := bson.M{
			"line_uid": result.ID.LineUID,
			"shop_id":  result.ID.ShopID,
		}
		update := bson.M{
			"$set": bson.M{
				"point_balance": balance,
				"shop_name":     result.ShopName,
				"updated_at":    time.Now(),
			},
			"$setOnInsert": bson.M{
				"line_uid":   result.ID.LineUID,
				"shop_id":    result.ID.ShopID,
				"created_at": time.Now(),
			},
		}
		opts := options.Update().SetUpsert(true)
		_, err := s.membersColl.UpdateOne(ctx, memberFilter, update, opts)
		if err == nil {
			count++
		}
	}

	return count, nil
}
