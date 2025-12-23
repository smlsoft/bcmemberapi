package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type LoginCode struct {
	Code        string    `bson:"code" json:"code"`
	ShopID      string    `bson:"shop_id,omitempty" json:"shop_id,omitempty"`
	Type        string    `bson:"type" json:"type"`     // "code" or "qr"
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

// Member represents a LINE member with central point balance (not per-shop)
type Member struct {
	LineUID      string    `bson:"line_uid" json:"line_uid"`
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

// UpsertMember creates or updates a central member record (not per-shop)
func (s *Store) UpsertMember(ctx context.Context, lineUID, displayName, pictureURL string) error {
	filter := bson.M{
		"line_uid": lineUID,
	}
	update := bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
		"$setOnInsert": bson.M{
			"line_uid":      lineUID,
			"display_name":  displayName,
			"picture_url":   pictureURL,
			"point_balance": 0,
			"created_at":    time.Now(),
		},
	}
	// Update display_name and picture_url only if provided
	if displayName != "" {
		update["$set"].(bson.M)["display_name"] = displayName
	}
	if pictureURL != "" {
		update["$set"].(bson.M)["picture_url"] = pictureURL
	}
	opts := options.Update().SetUpsert(true)
	_, err := s.membersColl.UpdateOne(ctx, filter, update, opts)
	return err
}

// UpdateMemberPoints updates a member's central point balance
func (s *Store) UpdateMemberPoints(ctx context.Context, lineUID string, pointChange float64) (float64, error) {
	filter := bson.M{
		"line_uid": lineUID,
	}
	update := bson.M{
		"$inc": bson.M{"point_balance": pointChange},
		"$set": bson.M{"updated_at": time.Now()},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After).SetUpsert(true)

	var member Member
	err := s.membersColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&member)
	if err != nil {
		return 0, err
	}
	return member.PointBalance, nil
}

// GetMemberByLineUID retrieves the central member record for a LINE user
func (s *Store) GetMemberByLineUID(ctx context.Context, lineUID string) (*Member, error) {
	var member Member
	err := s.membersColl.FindOne(ctx, bson.M{"line_uid": lineUID}).Decode(&member)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found
		}
		return nil, err
	}
	return &member, nil
}

// GetPointTransactionsByLineUID retrieves all point transactions for a LINE user (sorted by date desc)
func (s *Store) GetPointTransactionsByLineUID(ctx context.Context, lineUID string) ([]PointTransaction, error) {
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(50)
	cursor, err := s.pointTransColl.Find(ctx, bson.M{"line_uid": lineUID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var transactions []PointTransaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, err
	}
	return transactions, nil
}

// RecalculatePoints recalculates central point balance from transactions
func (s *Store) RecalculatePoints(ctx context.Context, lineUID, shopID string) (int, error) {
	// Build filter for transactions
	filter := bson.M{}
	if lineUID != "" {
		filter["line_uid"] = lineUID
	}
	// shopID is ignored for central system, but kept for backward compatibility

	// Aggregate to calculate total points per LINE user (central)
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$group", Value: bson.M{
			"_id":       "$line_uid",
			"total_get": bson.M{"$sum": "$get_point"},
			"total_use": bson.M{"$sum": "$use_point"},
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
			LineUID  string  `bson:"_id"`
			TotalGet float64 `bson:"total_get"`
			TotalUse float64 `bson:"total_use"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}

		balance := result.TotalGet - result.TotalUse

		// Update central member's point balance
		memberFilter := bson.M{
			"line_uid": result.LineUID,
		}
		update := bson.M{
			"$set": bson.M{
				"point_balance": balance,
				"updated_at":    time.Now(),
			},
			"$setOnInsert": bson.M{
				"line_uid":   result.LineUID,
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

// GenerateQRSession generates a random session string for QR login
func (s *Store) GenerateQRSession(ctx context.Context, shopID string) (string, error) {
	// Generate 16-character alphanumeric session ID
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	session := make([]byte, 16)
	for i := range session {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		session[i] = charset[n.Int64()]
	}

	loginCode := LoginCode{
		Code:      string(session),
		ShopID:    shopID,
		Type:      "qr",
		Status:    "pending",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	_, err := s.coll.InsertOne(ctx, loginCode)
	if err != nil {
		return "", err
	}
	return string(session), nil
}

// VerifyLiffSession verifies and updates a QR session from LIFF
func (s *Store) VerifyLiffSession(ctx context.Context, sessionID, userID, displayName, pictureURL string) error {
	filter := bson.M{
		"code":       sessionID,
		"type":       "qr",
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

// ========== Admin Functions ==========

// Global store instance for admin handlers
var globalStore *Store

func GetGlobalStore() *Store {
	return globalStore
}

func SetGlobalStore(s *Store) {
	globalStore = s
}

// Admin data types
type AdminData struct {
	LineUID     string    `bson:"line_uid"`
	DisplayName string    `bson:"display_name"`
	PictureURL  string    `bson:"picture_url"`
	Role        string    `bson:"role"`
	Email       string    `bson:"email,omitempty"`
	Phone       string    `bson:"phone,omitempty"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

type ShopData struct {
	ID             string    `bson:"_id,omitempty"`
	Name           string    `bson:"name"`
	OwnerLineUID   string    `bson:"owner_line_uid"`
	PointRate      int       `bson:"point_rate"`        // ซื้อกี่บาท = 1 แต้ม (default: 25)
	MinAmount      int       `bson:"min_amount"`        // ยอดขั้นต่ำที่จะได้แต้ม (default: 0)
	MaxPointsPerTx int       `bson:"max_points_per_tx"` // แต้มสูงสุดต่อบิล (0 = ไม่จำกัด)
	RedeemRate     int       `bson:"redeem_rate"`       // 1 แต้ม = กี่บาท (default: 1)
	Status         string    `bson:"status"`
	APIKey         string    `bson:"api_key"`
	Branches       int       `bson:"branches"`
	Members        int       `bson:"members"`
	PointsEarned   int       `bson:"points_earned"`
	PointsUsed     int       `bson:"points_used"`
	CreatedAt      time.Time `bson:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at"`
}

type DashboardStats struct {
	TotalMembers       int
	TotalShops         int
	PointsEarnedToday  int
	PointsUsedToday    int
	WeeklyData         []WeeklyDataPoint
	TopShops           []TopShopData
	RecentTransactions []RecentTxData
}

type WeeklyDataPoint struct {
	Label  string
	Earned int
	Used   int
}

type TopShopData struct {
	ID      string
	Name    string
	Members int
	Points  int
}

type RecentTxData struct {
	ID            string
	MemberName    string
	MemberPicture string
	Shop          string
	Type          string
	Points        int
	Time          string
}

type MemberData struct {
	ID            string
	LineUID       string
	DisplayName   string
	PictureURL    string
	Shop          string
	CurrentPoints int
	TotalEarned   int
	JoinedAt      string
	LastActive    string
}

type TransactionData struct {
	ID            string
	TransactionID string
	MemberName    string
	MemberPicture string
	Shop          string
	Type          string
	Points        int
	Amount        int
	Date          string
}

type TodayStatsData struct {
	Count  int
	Earned int
	Used   int
	Net    int
}

// SaveAdmin saves or updates an admin user
func SaveAdmin(lineUID, displayName, pictureURL, role string) error {
	s := GetGlobalStore()
	if s == nil {
		return nil // No store configured
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adminsColl := s.client.Database("bcmember").Collection("admins")

	filter := bson.M{"line_uid": lineUID}
	update := bson.M{
		"$set": bson.M{
			"display_name": displayName,
			"picture_url":  pictureURL,
			"role":         role,
			"updated_at":   time.Now(),
		},
		"$setOnInsert": bson.M{
			"line_uid":   lineUID,
			"created_at": time.Now(),
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := adminsColl.UpdateOne(ctx, filter, update, opts)
	return err
}

// GetDashboardStats returns dashboard statistics
func GetDashboardStats() (*DashboardStats, error) {
	s := GetGlobalStore()
	if s == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stats := &DashboardStats{}

	// Count members
	memberCount, _ := s.membersColl.CountDocuments(ctx, bson.M{})
	stats.TotalMembers = int(memberCount)

	// Count shops
	shopsColl := s.client.Database("bcmember").Collection("shops")
	shopCount, _ := shopsColl.CountDocuments(ctx, bson.M{})
	stats.TotalShops = int(shopCount)

	// Today's points
	today := time.Now().Truncate(24 * time.Hour)
	todayFilter := bson.M{
		"created_at": bson.M{"$gte": today},
	}

	cursor, err := s.pointTransColl.Find(ctx, todayFilter)
	if err == nil {
		defer cursor.Close(ctx)
		var transactions []PointTransaction
		cursor.All(ctx, &transactions)

		for _, tx := range transactions {
			stats.PointsEarnedToday += int(tx.GetPoint)
			stats.PointsUsedToday += int(tx.UsePoint)
		}
	}

	return stats, nil
}

// GetAllShops returns all shops
func GetAllShops() ([]ShopData, error) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return []ShopData{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(ctx)

	shopsColl := client.Database("bcmember").Collection("shops")
	cursor, err := shopsColl.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var shops []ShopData
	if err := cursor.All(ctx, &shops); err != nil {
		return nil, err
	}

	return shops, nil
}

// CreateShop creates a new shop (accepts ShopData pointer)
func CreateShop(shop *ShopData) (string, error) {
	// Get MongoDB URI from environment
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return "", fmt.Errorf("MONGODB_URI not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return "", fmt.Errorf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	// Set defaults if not provided
	if shop.ID == "" {
		shop.ID = generateRandomString(24)
	}
	if shop.Status == "" {
		shop.Status = "active"
	}
	shop.Branches = 1
	shop.Members = 0
	shop.PointsEarned = 0
	shop.PointsUsed = 0
	if shop.CreatedAt.IsZero() {
		shop.CreatedAt = time.Now()
	}
	shop.UpdatedAt = time.Now()

	shopsColl := client.Database("bcmember").Collection("shops")
	_, err = shopsColl.InsertOne(ctx, shop)
	if err != nil {
		return "", err
	}

	return shop.ID, nil
}

// UpdateShop updates shop settings
func UpdateShop(shopID string, updates map[string]interface{}) error {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return fmt.Errorf("MONGODB_URI not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	shopsColl := client.Database("bcmember").Collection("shops")

	updates["updated_at"] = time.Now()
	update := bson.M{"$set": updates}

	_, err = shopsColl.UpdateOne(ctx, bson.M{"_id": shopID}, update)
	return err
}

// CalculatePoints calculates points based on shop config
func CalculatePoints(shop *ShopData, amount float64) int {
	if shop.PointRate <= 0 {
		shop.PointRate = 25 // Default
	}

	// Check minimum amount
	if shop.MinAmount > 0 && int(amount) < shop.MinAmount {
		return 0
	}

	// Calculate points
	points := int(amount) / shop.PointRate

	// Apply max limit
	if shop.MaxPointsPerTx > 0 && points > shop.MaxPointsPerTx {
		points = shop.MaxPointsPerTx
	}

	return points
}

// GetShopByAPIKey returns shop data by API key
func GetShopByAPIKey(apiKey string) (*ShopData, error) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return nil, fmt.Errorf("MONGODB_URI not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	shopsColl := client.Database("bcmember").Collection("shops")

	var shop ShopData
	err = shopsColl.FindOne(ctx, bson.M{"api_key": apiKey}).Decode(&shop)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Not found
		}
		return nil, err
	}

	return &shop, nil
}

// GetMemberShops returns all shops where a member has points
func GetMemberShops(lineUID string) ([]MemberShopInfo, error) {
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return nil, fmt.Errorf("MONGODB_URI not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(ctx)

	// Get unique shop names from point_transactions for this member
	pointTransColl := client.Database("bcmember").Collection("point_transactions")

	pipeline := []bson.M{
		{"$match": bson.M{"line_uid": lineUID}},
		{"$group": bson.M{
			"_id":        "$shop_name",
			"shop_id":    bson.M{"$first": "$shop_id"},
			"get_point":  bson.M{"$sum": "$get_point"},
			"use_point":  bson.M{"$sum": "$use_point"},
			"last_visit": bson.M{"$max": "$created_at"},
		}},
	}

	cursor, err := pointTransColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []MemberShopInfo
	for cursor.Next(ctx) {
		var item struct {
			ShopName  string    `bson:"_id"`
			ShopID    string    `bson:"shop_id"`
			GetPoint  float64   `bson:"get_point"`
			UsePoint  float64   `bson:"use_point"`
			LastVisit time.Time `bson:"last_visit"`
		}
		if err := cursor.Decode(&item); err != nil {
			continue
		}
		results = append(results, MemberShopInfo{
			ShopID:       item.ShopID,
			ShopName:     item.ShopName,
			PointBalance: item.GetPoint - item.UsePoint,
			LastVisit:    item.LastVisit,
		})
	}

	return results, nil
}

// MemberShopInfo represents shop info for a member
type MemberShopInfo struct {
	ShopID       string    `json:"shop_id"`
	ShopName     string    `json:"shop_name"`
	PointBalance float64   `json:"point_balance"`
	LastVisit    time.Time `json:"last_visit"`
}

// generateRandomString generates a random string of given length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}

// GetAllMembers returns all members for admin view
func GetAllMembers() ([]MemberData, error) {
	s := GetGlobalStore()
	if s == nil {
		return nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := s.membersColl.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var members []Member
	if err := cursor.All(ctx, &members); err != nil {
		return nil, err
	}

	result := make([]MemberData, 0, len(members))
	for _, m := range members {
		result = append(result, MemberData{
			ID:            m.LineUID,
			LineUID:       m.LineUID,
			DisplayName:   m.DisplayName,
			PictureURL:    m.PictureURL,
			Shop:          "Central", // Central system - no shop specific
			CurrentPoints: int(m.PointBalance),
			TotalEarned:   int(m.PointBalance), // Simplified
			JoinedAt:      m.CreatedAt.Format("2 Jan 2006"),
			LastActive:    m.UpdatedAt.Format("2 Jan 2006"),
		})
	}

	return result, nil
}

// GetAllTransactions returns all transactions for admin view
func GetAllTransactions() ([]TransactionData, *TodayStatsData, error) {
	s := GetGlobalStore()
	if s == nil {
		return nil, nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(100)
	cursor, err := s.pointTransColl.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, nil, err
	}
	defer cursor.Close(ctx)

	var transactions []PointTransaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, nil, err
	}

	result := make([]TransactionData, 0, len(transactions))
	todayStats := &TodayStatsData{}
	today := time.Now().Truncate(24 * time.Hour)

	for i, tx := range transactions {
		txType := "earn"
		points := int(tx.GetPoint)
		if tx.UsePoint > 0 {
			txType = "redeem"
			points = -int(tx.UsePoint)
		}

		result = append(result, TransactionData{
			ID:            tx.DocNo,
			TransactionID: tx.DocNo,
			MemberName:    tx.LineUID[:8] + "...",
			MemberPicture: "https://via.placeholder.com/32",
			Shop:          tx.ShopName,
			Type:          txType,
			Points:        points,
			Amount:        int(tx.GetPoint) * 25, // Estimate
			Date:          tx.CreatedAt.Format(time.RFC3339),
		})

		// Today stats
		if tx.CreatedAt.After(today) {
			todayStats.Count++
			todayStats.Earned += int(tx.GetPoint)
			todayStats.Used += int(tx.UsePoint)
		}

		_ = i
	}

	todayStats.Net = todayStats.Earned - todayStats.Used

	return result, todayStats, nil
}

// ========== Login Session Functions (for QR Code login) ==========

// LoginSession represents a login session for QR code authentication
type LoginSession struct {
	SessionID string     `bson:"session_id" json:"session_id"`
	Type      string     `bson:"type" json:"type"` // "member" or "admin"
	ShopID    string     `bson:"shop_id,omitempty" json:"shop_id,omitempty"`
	Status    string     `bson:"status" json:"status"` // "pending", "success"
	User      *LoginUser `bson:"user,omitempty" json:"user,omitempty"`
	CreatedAt time.Time  `bson:"created_at" json:"created_at"`
	ExpiresAt time.Time  `bson:"expires_at" json:"expires_at"`
}

// LoginUser represents the user data stored in a login session
type LoginUser struct {
	LineUserID  string `bson:"line_user_id" json:"line_user_id"`
	DisplayName string `bson:"display_name" json:"display_name"`
	PictureURL  string `bson:"picture_url" json:"picture_url"`
}

// GenerateLoginSession creates a new login session for QR authentication
func (s *Store) GenerateLoginSession(ctx context.Context, sessionType, shopID string) (string, error) {
	// Generate 20-character alphanumeric session ID
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	sessionBytes := make([]byte, 20)
	for i := range sessionBytes {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		sessionBytes[i] = charset[n.Int64()]
	}
	sessionID := string(sessionBytes)

	session := LoginSession{
		SessionID: sessionID,
		Type:      sessionType,
		ShopID:    shopID,
		Status:    "pending",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	db := s.client.Database("bcmember")
	coll := db.Collection("login_sessions")
	_, err := coll.InsertOne(ctx, session)
	if err != nil {
		return "", err
	}
	return sessionID, nil
}

// GetLoginSession retrieves a login session by ID
func (s *Store) GetLoginSession(ctx context.Context, sessionID string) (*LoginSession, error) {
	db := s.client.Database("bcmember")
	coll := db.Collection("login_sessions")

	var session LoginSession
	err := coll.FindOne(ctx, bson.M{"session_id": sessionID}).Decode(&session)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// VerifyLoginSession updates a login session with user data
func (s *Store) VerifyLoginSession(ctx context.Context, sessionID string, user *LoginUser) error {
	db := s.client.Database("bcmember")
	coll := db.Collection("login_sessions")

	filter := bson.M{
		"session_id": sessionID,
		"status":     "pending",
		"expires_at": bson.M{"$gt": time.Now()},
	}
	update := bson.M{
		"$set": bson.M{
			"status": "success",
			"user":   user,
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
