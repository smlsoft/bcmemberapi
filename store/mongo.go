package store

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
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
	LineUID       string    `bson:"line_uid" json:"line_uid"`
	ShopID        string    `bson:"shop_id" json:"shop_id"`
	ShopName      string    `bson:"shop_name" json:"shop_name"`
	DocNo         string    `bson:"doc_no" json:"doc_no"`
	GetPoint      float64   `bson:"get_point" json:"get_point"`
	UsePoint      float64   `bson:"use_point" json:"use_point"`
	Note          string    `bson:"note,omitempty" json:"note,omitempty"`
	Source        string    `bson:"source,omitempty" json:"source,omitempty"`
	BalanceBefore float64   `bson:"balance_before,omitempty" json:"balance_before,omitempty"`
	BalanceAfter  float64   `bson:"balance_after,omitempty" json:"balance_after,omitempty"`
	CreatedAt     time.Time `bson:"created_at" json:"created_at"`
}

// Member represents a LINE member with central point balance (not per-shop)
type Member struct {
	LineUID      string    `bson:"line_uid" json:"line_uid"`
	DisplayName  string    `bson:"display_name" json:"display_name"`
	PictureURL   string    `bson:"picture_url" json:"picture_url"`
	PointBalance float64   `bson:"point_balance" json:"point_balance"`
	TotalEarned  float64   `bson:"total_earned" json:"total_earned"`
	Tier         string    `bson:"tier" json:"tier"`
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at" json:"updated_at"`
}

// AuditLog represents an admin action log entry
type AuditLog struct {
	AdminLineUID string    `bson:"admin_line_uid" json:"admin_line_uid"`
	AdminName    string    `bson:"admin_name" json:"admin_name"`
	Action       string    `bson:"action" json:"action"`
	TargetType   string    `bson:"target_type" json:"target_type"`
	TargetID     string    `bson:"target_id" json:"target_id"`
	Details      string    `bson:"details" json:"details"`
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
}

// AuditLogFilter for querying audit logs
type AuditLogFilter struct {
	Action    string
	StartDate string
	EndDate   string
	Page      int
	PageSize  int
}

// CalculateTier returns the tier name based on lifetime total earned points
func CalculateTier(totalEarned float64) string {
	switch {
	case totalEarned >= 5000:
		return "Platinum"
	case totalEarned >= 2000:
		return "Gold"
	case totalEarned >= 500:
		return "Silver"
	default:
		return "Standard"
	}
}

type Store struct {
	client         *mongo.Client
	coll           *mongo.Collection
	chatHistColl   *mongo.Collection
	pointTransColl *mongo.Collection
	membersColl    *mongo.Collection
	tableOrderColl *mongo.Collection
}

// formatTimeAgo returns a human-readable time difference in Thai
func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)

	if diff < time.Minute {
		return "เมื่อกี้"
	} else if diff < time.Hour {
		mins := int(diff.Minutes())
		return fmt.Sprintf("%d นาทีที่แล้ว", mins)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return fmt.Sprintf("%d ชั่วโมงที่แล้ว", hours)
	} else {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d วันที่แล้ว", days)
	}
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
	tableOrderCollInst := db.Collection("table_order_sessions")

	s := &Store{
		client:         client,
		coll:           coll,
		chatHistColl:   chatHistColl,
		pointTransColl: pointTransColl,
		membersColl:    membersColl,
		tableOrderColl: tableOrderCollInst,
	}

	// Ensure indexes (runs once, idempotent)
	s.ensureIndexes(ctx)
	EnsureTableOrderIndexes(ctx, client)

	return s, nil
}

// ensureIndexes creates necessary indexes for data integrity
func (s *Store) ensureIndexes(ctx context.Context) {
	// Unique index on doc_no + shop_id in point_transactions to prevent duplicates
	_, err := s.pointTransColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "doc_no", Value: 1},
			{Key: "shop_id", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("idx_doc_no_shop_id_unique"),
	})
	if err != nil {
		// Log but don't fail - index may already exist
		fmt.Printf("INFO: ensure index doc_no+shop_id: %v\n", err)
	}

	// Index on line_uid in members for fast lookup
	_, err = s.membersColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "line_uid", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("idx_line_uid_unique"),
	})
	if err != nil {
		fmt.Printf("INFO: ensure index line_uid: %v\n", err)
	}

	// Index on created_at in audit_logs for sorting and date range queries
	auditLogsColl := s.client.Database("bcmember").Collection("audit_logs")
	_, err = auditLogsColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "created_at", Value: -1}},
		Options: options.Index().SetName("idx_audit_logs_created_at"),
	})
	if err != nil {
		fmt.Printf("INFO: ensure index audit_logs created_at: %v\n", err)
	}
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

// ErrDuplicateTransaction indicates a duplicate doc_no + shop_id
var ErrDuplicateTransaction = fmt.Errorf("duplicate transaction")

// SavePointTransaction saves a point transaction to the database
// Returns ErrDuplicateTransaction if doc_no + shop_id already exists
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
	if err != nil {
		// Check for duplicate key error
		if mongo.IsDuplicateKeyError(err) {
			return ErrDuplicateTransaction
		}
		return err
	}
	return nil
}

// UpsertMember creates or updates a central member record (not per-shop)
func (s *Store) UpsertMember(ctx context.Context, lineUID, displayName, pictureURL string) error {
	lineUID = strings.TrimSpace(lineUID)
	displayName = strings.TrimSpace(displayName)
	pictureURL = strings.TrimSpace(pictureURL)
	if lineUID == "" {
		return fmt.Errorf("line_uid is required")
	}

	now := time.Now()
	setFields := bson.M{"updated_at": now}
	if displayName != "" {
		setFields["display_name"] = displayName
	}
	if pictureURL != "" {
		setFields["picture_url"] = pictureURL
	}

	update := bson.M{
		"$set": setFields,
		"$setOnInsert": bson.M{
			"line_uid":      lineUID,
			"point_balance": 0,
			"total_earned":  0,
			"tier":          "Standard",
			"created_at":    now,
		},
	}
	opts := options.Update().SetUpsert(true)
	_, err := s.membersColl.UpdateOne(ctx, bson.M{"line_uid": lineUID}, update, opts)
	return err
}

// UpdateMemberPoints updates a member's central point balance and tier
func (s *Store) LogInvoiceIssue(ctx context.Context, shopID, shopName, docNo, lineUID, issueType, reason string) error {
	issuesColl := s.client.Database("bcmember").Collection("invoice_issues")
	_, err := issuesColl.InsertOne(ctx, invoiceIssueRecord{
		ShopID:    shopID,
		ShopName:  shopName,
		DocNo:     docNo,
		LineUID:   lineUID,
		IssueType: issueType,
		Reason:    reason,
		CreatedAt: time.Now(),
	})
	return err
}
func (s *Store) UpdateMemberPoints(ctx context.Context, lineUID string, pointChange float64, getPoint float64) (float64, string, error) {
	filter := bson.M{
		"line_uid": lineUID,
	}
	incFields := bson.M{"point_balance": pointChange}
	if getPoint > 0 {
		incFields["total_earned"] = getPoint
	}
	update := bson.M{
		"$inc": incFields,
		"$set": bson.M{"updated_at": time.Now()},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After).SetUpsert(true)

	var member Member
	err := s.membersColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&member)
	if err != nil {
		return 0, "", err
	}

	// Recalculate tier if needed
	newTier := CalculateTier(member.TotalEarned)
	if member.Tier != newTier {
		s.membersColl.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"tier": newTier}})
		member.Tier = newTier
	}

	return member.PointBalance, member.Tier, nil
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
		tier := CalculateTier(result.TotalGet)

		// Update central member's point balance, total_earned, and tier
		memberFilter := bson.M{
			"line_uid": result.LineUID,
		}
		update := bson.M{
			"$set": bson.M{
				"point_balance": balance,
				"total_earned":  result.TotalGet,
				"tier":          tier,
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

// Global store instance - singleton for all handlers
var globalStore *Store

func GetGlobalStore() *Store {
	return globalStore
}

func SetGlobalStore(s *Store) {
	globalStore = s
}

// getOrCreateStore returns the global store or creates a new one
func getOrCreateStore() (*Store, error) {
	if globalStore != nil {
		return globalStore, nil
	}
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		return nil, fmt.Errorf("MONGODB_URI not set")
	}
	s, err := NewStore(mongoURI)
	if err != nil {
		return nil, err
	}
	globalStore = s
	return s, nil
}

// Admin data types
type AdminData struct {
	LineUID     string    `bson:"line_uid" json:"line_uid"`
	DisplayName string    `bson:"display_name" json:"display_name"`
	PictureURL  string    `bson:"picture_url" json:"picture_url"`
	Role        string    `bson:"role" json:"role"`
	Email       string    `bson:"email,omitempty" json:"email,omitempty"`
	Phone       string    `bson:"phone,omitempty" json:"phone,omitempty"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at" json:"updated_at"`
}

type ShopData struct {
	ID             string    `bson:"_id,omitempty" json:"_id,omitempty"`
	Name           string    `bson:"name" json:"name"`
	OwnerLineUID   string    `bson:"owner_line_uid" json:"owner_line_uid"`
	PointRate      int       `bson:"point_rate" json:"point_rate"`
	MinAmount      int       `bson:"min_amount" json:"min_amount"`
	MaxPointsPerTx int       `bson:"max_points_per_tx" json:"max_points_per_tx"`
	RedeemRate     int       `bson:"redeem_rate" json:"redeem_rate"`
	Status         string    `bson:"status" json:"status"`
	APIKey         string    `bson:"api_key" json:"api_key"`
	Branches       int       `bson:"branches" json:"branches"`
	Members        int       `bson:"members" json:"members"`
	PointsEarned   int       `bson:"points_earned" json:"points_earned"`
	PointsUsed     int       `bson:"points_used" json:"points_used"`
	CreatedAt      time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at" json:"updated_at"`
}

type DashboardStats struct {
	TotalMembers           int                `json:"total_members"`
	TotalShops             int                `json:"total_shops"`
	PointsEarnedToday      int                `json:"points_earned_today"`
	PointsUsedToday        int                `json:"points_used_today"`
	OutstandingPoints      int                `json:"outstanding_points"`
	ShopsStats             []ShopPointsStats  `json:"shops_stats"`
	RecentTransactions     []RecentTxData     `json:"recent_transactions"`
	TierStats              map[string]int     `json:"tier_stats"`
	TopMembers             []MemberData       `json:"top_members"`
	AbnormalAdjustments    []TransactionData  `json:"abnormal_adjustments"`
	DuplicateInvoicesToday int                `json:"duplicate_invoices_today"`
	RejectedInvoicesToday  int                `json:"rejected_invoices_today"`
	RecentInvoiceIssues    []InvoiceIssueData `json:"recent_invoice_issues"`
}

type InvoiceIssueData struct {
	ID        string `json:"id" bson:"_id,omitempty"`
	ShopID    string `json:"shop_id" bson:"shop_id"`
	ShopName  string `json:"shop_name" bson:"shop_name"`
	DocNo     string `json:"doc_no" bson:"doc_no"`
	LineUID   string `json:"line_uid" bson:"line_uid"`
	IssueType string `json:"issue_type" bson:"issue_type"`
	Reason    string `json:"reason" bson:"reason"`
	CreatedAt string `json:"created_at" bson:"-"`
}

type invoiceIssueRecord struct {
	ShopID    string    `bson:"shop_id"`
	ShopName  string    `bson:"shop_name"`
	DocNo     string    `bson:"doc_no"`
	LineUID   string    `bson:"line_uid"`
	IssueType string    `bson:"issue_type"`
	Reason    string    `bson:"reason"`
	CreatedAt time.Time `bson:"created_at"`
}
type ShopPointsStats struct {
	ShopID       string `json:"shop_id" bson:"_id"`
	ShopName     string `json:"shop_name" bson:"shop_name"`
	PointsEarned int    `json:"points_earned" bson:"points_earned"`
	PointsUsed   int    `json:"points_used" bson:"points_used"`
}

type RecentTxData struct {
	ID            string
	MemberUID     string
	MemberName    string
	MemberPicture string
	Shop          string
	Type          string
	Points        int
	Time          string
}

type MemberData struct {
	ID            string   `json:"id"`
	LineUID       string   `json:"line_uid"`
	DisplayName   string   `json:"display_name"`
	PictureURL    string   `json:"picture_url"`
	ShopsVisited  []string `json:"shops_visited"`
	CurrentPoints int      `json:"current_points"`
	TotalEarned   int      `json:"total_earned"`
	Tier          string   `json:"tier"`
	JoinedAt      string   `json:"joined_at"`
	LastActive    string   `json:"last_active"`
}

type TransactionData struct {
	ID            string  `json:"id"`
	TransactionID string  `json:"transaction_id"`
	MemberUID     string  `json:"member_uid"`
	MemberName    string  `json:"member_name"`
	MemberPicture string  `json:"member_picture"`
	ShopID        string  `json:"shop_id"`
	Shop          string  `json:"shop"`
	Type          string  `json:"type"`
	Points        int     `json:"points"`
	GetPoint      float64 `json:"get_point"`
	UsePoint      float64 `json:"use_point"`
	Amount        int     `json:"amount"`
	Date          string  `json:"date"`
	Note          string  `json:"note,omitempty"`
	Source        string  `json:"source,omitempty"`
	BalanceBefore float64 `json:"balance_before,omitempty"`
	BalanceAfter  float64 `json:"balance_after,omitempty"`
}

type ShopMemberSummary struct {
	LineUID          string    `json:"line_uid"`
	DisplayName      string    `json:"display_name"`
	PictureURL       string    `json:"picture_url"`
	PointsEarned     float64   `json:"points_earned"`
	PointsUsed       float64   `json:"points_used"`
	PointBalance     float64   `json:"point_balance"`
	TransactionCount int       `json:"transaction_count"`
	LastVisit        time.Time `json:"last_visit"`
}

type ShopDetailStats struct {
	MemberCount      int     `json:"member_count"`
	TransactionCount int     `json:"transaction_count"`
	PointsEarned     float64 `json:"points_earned"`
	PointsUsed       float64 `json:"points_used"`
	PointBalance     float64 `json:"point_balance"`
}

type ShopDetailData struct {
	Stats              ShopDetailStats     `json:"stats"`
	Members            []ShopMemberSummary `json:"members"`
	TopMembers         []ShopMemberSummary `json:"top_members"`
	RecentTransactions []TransactionData   `json:"recent_transactions"`
	InvoiceIssues      []InvoiceIssueData  `json:"invoice_issues"`
}
type TodayStatsData struct {
	Count  int
	Earned int
	Used   int
	Net    int
}

type MemberFilter struct {
	Query    string
	ShopID   string
	Tier     string
	SortBy   string
	Page     int
	PageSize int
}

type MemberListResult struct {
	Members     []MemberData `json:"members"`
	Total       int          `json:"total"`
	Page        int          `json:"page"`
	PageSize    int          `json:"page_size"`
	TotalPoints int          `json:"total_points"`
}

type MemberDetailData struct {
	Member       MemberData        `json:"member"`
	Shops        []MemberShopInfo  `json:"shops"`
	Transactions []TransactionData `json:"transactions"`
}

type AdjustPointResult struct {
	NewBalance      float64 `json:"new_balance"`
	PreviousBalance float64 `json:"previous_balance"`
	TransactionID   string  `json:"transaction_id"`
}

type PointRulePreview struct {
	Amount      float64 `json:"amount"`
	GetPoint    int     `json:"get_point"`
	RedeemPoint float64 `json:"redeem_point"`
	RedeemValue float64 `json:"redeem_value"`
}

// SaveAdmin saves or updates an admin user
func SaveAdmin(lineUID, displayName, pictureURL, role string) error {
	s, err := getOrCreateStore()
	if err != nil {
		return err
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
	_, err = adminsColl.UpdateOne(ctx, filter, update, opts)
	return err
}

// CountAdmins returns the number of admins in the database
func CountAdmins() (int64, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adminsColl := s.client.Database("bcmember").Collection("admins")
	return adminsColl.CountDocuments(ctx, bson.M{})
}

// GetAdminRole checks if a LINE user is an admin and returns their role
func GetAdminRole(lineUID string) (string, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adminsColl := s.client.Database("bcmember").Collection("admins")

	var result struct {
		Role string `bson:"role"`
	}
	err = adminsColl.FindOne(ctx, bson.M{"line_uid": lineUID}).Decode(&result)
	if err != nil {
		return "", err
	}
	return result.Role, nil
}

// GetAllAdmins returns all admin users
func GetAllAdmins() ([]AdminData, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adminsColl := s.client.Database("bcmember").Collection("admins")

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cursor, err := adminsColl.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var admins []AdminData
	if err := cursor.All(ctx, &admins); err != nil {
		return nil, err
	}
	return admins, nil
}

// DeleteAdmin removes an admin by LINE UID
func DeleteAdmin(lineUID string) error {
	s, err := getOrCreateStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	adminsColl := s.client.Database("bcmember").Collection("admins")
	_, err = adminsColl.DeleteOne(ctx, bson.M{"line_uid": lineUID})
	return err
}

// GetDashboardStats returns dashboard statistics
func GetDashboardStats() (*DashboardStats, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db := s.client.Database("bcmember")
	membersColl := db.Collection("members")
	shopsColl := db.Collection("shops")
	pointTransColl := s.pointTransColl

	stats := &DashboardStats{}

	// Count members
	memberCount, _ := membersColl.CountDocuments(ctx, bson.M{})
	stats.TotalMembers = int(memberCount)

	// Tier stats
	stats.TierStats = map[string]int{"Standard": 0, "Silver": 0, "Gold": 0, "Platinum": 0}
	tierPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":   "$tier",
			"count": bson.M{"$sum": 1},
		}}},
	}
	tierCursor, tierErr := membersColl.Aggregate(ctx, tierPipeline)
	if tierErr == nil {
		defer tierCursor.Close(ctx)
		for tierCursor.Next(ctx) {
			var row struct {
				Tier  string `bson:"_id"`
				Count int    `bson:"count"`
			}
			if tierCursor.Decode(&row) == nil {
				tier := row.Tier
				if tier == "" {
					tier = "Standard"
				}
				stats.TierStats[tier] = row.Count
			}
		}
	}

	// Outstanding points and top members
	outstandingPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{"_id": nil, "points": bson.M{"$sum": "$point_balance"}}}},
	}
	if outCursor, err := membersColl.Aggregate(ctx, outstandingPipeline); err == nil {
		defer outCursor.Close(ctx)
		if outCursor.Next(ctx) {
			var row struct {
				Points float64 `bson:"points"`
			}
			if outCursor.Decode(&row) == nil {
				stats.OutstandingPoints = int(row.Points)
			}
		}
	}

	topOpts := options.Find().SetSort(bson.D{{Key: "point_balance", Value: -1}}).SetLimit(10)
	if topCursor, err := membersColl.Find(ctx, bson.M{}, topOpts); err == nil {
		defer topCursor.Close(ctx)
		var topMembers []Member
		if topCursor.All(ctx, &topMembers) == nil {
			stats.TopMembers = make([]MemberData, 0, len(topMembers))
			for _, m := range topMembers {
				stats.TopMembers = append(stats.TopMembers, memberDataFromMember(m))
			}
		}
	}

	abnormalFilter := bson.M{"$or": []bson.M{{"source": "admin_adjust"}, {"shop_id": "admin"}, {"doc_no": bson.M{"$regex": "^ADJ_"}}}}
	abnormalOpts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(20)
	if abnormalCursor, err := pointTransColl.Find(ctx, abnormalFilter, abnormalOpts); err == nil {
		defer abnormalCursor.Close(ctx)
		var txs []PointTransaction
		if abnormalCursor.All(ctx, &txs) == nil {
			memberMap := make(map[string]*Member)
			lineUIDs := make([]string, 0, len(txs))
			for _, tx := range txs {
				if tx.LineUID != "" {
					memberMap[tx.LineUID] = nil
					lineUIDs = append(lineUIDs, tx.LineUID)
				}
			}
			if len(lineUIDs) > 0 {
				if mCursor, err := membersColl.Find(ctx, bson.M{"line_uid": bson.M{"$in": lineUIDs}}); err == nil {
					defer mCursor.Close(ctx)
					for mCursor.Next(ctx) {
						var m Member
						if mCursor.Decode(&m) == nil {
							memberMap[m.LineUID] = &m
						}
					}
				}
			}
			stats.AbnormalAdjustments = make([]TransactionData, 0, len(txs))
			for _, tx := range txs {
				stats.AbnormalAdjustments = append(stats.AbnormalAdjustments, transactionDataFromPointTransaction(tx, memberMap))
			}
		}
	}
	// Count shops
	shopCount, _ := shopsColl.CountDocuments(ctx, bson.M{})
	stats.TotalShops = int(shopCount)

	// Today's points
	today := time.Now().Truncate(24 * time.Hour)
	todayFilter := bson.M{
		"created_at": bson.M{"$gte": today},
	}

	cursor, err := pointTransColl.Find(ctx, todayFilter)
	if err == nil {
		defer cursor.Close(ctx)
		var transactions []PointTransaction
		cursor.All(ctx, &transactions)

		for _, tx := range transactions {
			stats.PointsEarnedToday += int(tx.GetPoint)
			stats.PointsUsedToday += int(tx.UsePoint)
		}
	}

	issuesColl := db.Collection("invoice_issues")
	createdToday := bson.M{"$gte": today}
	duplicateToday, _ := issuesColl.CountDocuments(ctx, bson.M{"issue_type": "duplicate", "created_at": createdToday})
	rejectedToday, _ := issuesColl.CountDocuments(ctx, bson.M{"issue_type": "rejected", "created_at": createdToday})
	stats.DuplicateInvoicesToday = int(duplicateToday)
	stats.RejectedInvoicesToday = int(rejectedToday)

	issueOpts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(20)
	if issueCursor, err := issuesColl.Find(ctx, bson.M{}, issueOpts); err == nil {
		defer issueCursor.Close(ctx)
		for issueCursor.Next(ctx) {
			var issue invoiceIssueRecord
			if issueCursor.Decode(&issue) == nil {
				stats.RecentInvoiceIssues = append(stats.RecentInvoiceIssues, InvoiceIssueData{
					ShopID:    issue.ShopID,
					ShopName:  issue.ShopName,
					DocNo:     issue.DocNo,
					LineUID:   issue.LineUID,
					IssueType: issue.IssueType,
					Reason:    issue.Reason,
					CreatedAt: issue.CreatedAt.Format(time.RFC3339),
				})
			}
		}
	}
	// Get shop points stats - aggregate by shop_id, sorted by points_earned desc
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":           "$shop_id",
				"shop_name":     bson.M{"$first": "$shop_name"},
				"points_earned": bson.M{"$sum": "$get_point"},
				"points_used":   bson.M{"$sum": "$use_point"},
			},
		},
		{
			"$sort": bson.M{"points_earned": -1},
		},
	}

	aggCursor, err := pointTransColl.Aggregate(ctx, pipeline)
	if err == nil {
		defer aggCursor.Close(ctx)
		var shopStats []ShopPointsStats
		if err := aggCursor.All(ctx, &shopStats); err == nil {
			stats.ShopsStats = shopStats
		}
	}

	// Get recent transactions (last 10)
	recentOpts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(10)
	recentCursor, err := pointTransColl.Find(ctx, bson.M{}, recentOpts)
	if err == nil {
		defer recentCursor.Close(ctx)
		var recentTxs []PointTransaction
		if err := recentCursor.All(ctx, &recentTxs); err == nil {
			// Get member info for each transaction
			for _, tx := range recentTxs {
				txType := "earn"
				points := int(tx.GetPoint)
				if tx.UsePoint > 0 {
					txType = "redeem"
					points = int(tx.UsePoint)
				}

				// Get member info
				memberName := "สมาชิก"
				memberPicture := ""
				var member Member
				if err := membersColl.FindOne(ctx, bson.M{"line_uid": tx.LineUID}).Decode(&member); err == nil {
					if member.DisplayName != "" {
						memberName = member.DisplayName
					} else if len(tx.LineUID) > 4 {
						memberName = "ผู้ใช้ " + tx.LineUID[len(tx.LineUID)-4:]
					}
					if member.PictureURL != "" {
						memberPicture = member.PictureURL
					}
				} else if len(tx.LineUID) > 4 {
					// Member not found, show partial line_uid
					memberName = "ผู้ใช้ " + tx.LineUID[len(tx.LineUID)-4:]
				}

				// Format time ago
				timeAgo := formatTimeAgo(tx.CreatedAt)

				stats.RecentTransactions = append(stats.RecentTransactions, RecentTxData{
					ID:            tx.DocNo,
					MemberUID:     tx.LineUID,
					MemberName:    memberName,
					MemberPicture: memberPicture,
					Shop:          tx.ShopName,
					Type:          txType,
					Points:        points,
					Time:          timeAgo,
				})
			}
		}
	}

	return stats, nil
}

// GetAllShops returns all shops
func GetAllShops() ([]ShopData, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	shopsColl := s.client.Database("bcmember").Collection("shops")
	cursor, err := shopsColl.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var shops []ShopData
	if err := cursor.All(ctx, &shops); err != nil {
		return nil, err
	}

	// Compute member count and points per shop from point_transactions
	// Use shop_name to match because shop_id in transactions is from POS (not MongoDB _id)
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":           "$shop_name",
				"members":       bson.M{"$addToSet": "$line_uid"},
				"points_earned": bson.M{"$sum": "$get_point"},
				"points_used":   bson.M{"$sum": "$use_point"},
			},
		},
	}
	aggCursor, err := s.pointTransColl.Aggregate(ctx, pipeline)
	if err == nil {
		defer aggCursor.Close(ctx)
		type shopAgg struct {
			ShopName     string   `bson:"_id"`
			Members      []string `bson:"members"`
			PointsEarned float64  `bson:"points_earned"`
			PointsUsed   float64  `bson:"points_used"`
		}
		statsMap := make(map[string]shopAgg)
		for aggCursor.Next(ctx) {
			var agg shopAgg
			if err := aggCursor.Decode(&agg); err == nil {
				statsMap[agg.ShopName] = agg
			}
		}
		for i := range shops {
			if agg, ok := statsMap[shops[i].Name]; ok {
				shops[i].Members = len(agg.Members)
				shops[i].PointsEarned = int(agg.PointsEarned)
				shops[i].PointsUsed = int(agg.PointsUsed)
			}
		}
	}

	return shops, nil
}

// CreateShop creates a new shop (accepts ShopData pointer)
func CreateShop(shop *ShopData) (string, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	shopsColl := s.client.Database("bcmember").Collection("shops")
	_, err = shopsColl.InsertOne(ctx, shop)
	if err != nil {
		return "", err
	}

	return shop.ID, nil
}

// UpdateShop updates shop settings
func UpdateShop(shopID string, updates map[string]interface{}) error {
	s, err := getOrCreateStore()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	shopsColl := s.client.Database("bcmember").Collection("shops")

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
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	shopsColl := s.client.Database("bcmember").Collection("shops")

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
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get unique shop names from point_transactions for this member
	pointTransColl := s.pointTransColl

	pipeline := []bson.M{
		{"$match": bson.M{"line_uid": lineUID}},
		{"$group": bson.M{
			"_id":        "$shop_name",
			"shop_id":    bson.M{"$first": "$shop_id"},
			"get_point":  bson.M{"$sum": "$get_point"},
			"use_point":  bson.M{"$sum": "$use_point"},
			"tx_count":   bson.M{"$sum": 1},
			"last_visit": bson.M{"$max": "$created_at"},
		}},
		{"$sort": bson.M{"last_visit": -1}},
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
			TxCount   int       `bson:"tx_count"`
			LastVisit time.Time `bson:"last_visit"`
		}
		if err := cursor.Decode(&item); err != nil {
			continue
		}
		results = append(results, MemberShopInfo{
			ShopID:           item.ShopID,
			ShopName:         item.ShopName,
			PointsEarned:     item.GetPoint,
			PointsUsed:       item.UsePoint,
			PointBalance:     item.GetPoint - item.UsePoint,
			TransactionCount: item.TxCount,
			LastVisit:        item.LastVisit,
		})
	}

	return results, nil
}

// MemberShopInfo represents shop info for a member
type MemberShopInfo struct {
	ShopID           string    `json:"shop_id"`
	ShopName         string    `json:"shop_name"`
	PointsEarned     float64   `json:"points_earned"`
	PointsUsed       float64   `json:"points_used"`
	PointBalance     float64   `json:"point_balance"`
	TransactionCount int       `json:"transaction_count"`
	LastVisit        time.Time `json:"last_visit"`
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

// GetAllMembers returns all members for admin view with shops visited and total earned
func GetAllMembers() ([]MemberData, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	membersColl := s.membersColl
	pointTransColl := s.pointTransColl

	cursor, err := membersColl.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var members []Member
	if err := cursor.All(ctx, &members); err != nil {
		return nil, err
	}

	// Build a map of line_uid -> member data
	result := make([]MemberData, 0, len(members))
	memberMap := make(map[string]*MemberData)

	for i, m := range members {
		tier := m.Tier
		if tier == "" {
			tier = CalculateTier(m.TotalEarned)
		}
		md := MemberData{
			ID:            m.LineUID,
			LineUID:       m.LineUID,
			DisplayName:   m.DisplayName,
			PictureURL:    m.PictureURL,
			ShopsVisited:  []string{},
			CurrentPoints: int(m.PointBalance),
			TotalEarned:   int(m.TotalEarned),
			Tier:          tier,
			JoinedAt:      m.CreatedAt.Format("2 Jan 2006"),
			LastActive:    m.UpdatedAt.Format("2 Jan 2006"),
		}
		result = append(result, md)
		memberMap[m.LineUID] = &result[i]
	}

	// Aggregate transactions to get shops visited and total earned per member
	pipeline := []bson.M{
		{
			"$group": bson.M{
				"_id":          "$line_uid",
				"shops":        bson.M{"$addToSet": "$shop_name"},
				"total_earned": bson.M{"$sum": "$get_point"},
			},
		},
	}

	aggCursor, err := pointTransColl.Aggregate(ctx, pipeline)
	if err == nil {
		defer aggCursor.Close(ctx)

		type MemberAgg struct {
			LineUID     string   `bson:"_id"`
			Shops       []string `bson:"shops"`
			TotalEarned float64  `bson:"total_earned"`
		}

		var aggResults []MemberAgg
		if err := aggCursor.All(ctx, &aggResults); err == nil {
			for _, agg := range aggResults {
				if md, ok := memberMap[agg.LineUID]; ok {
					md.ShopsVisited = agg.Shops
					md.TotalEarned = int(agg.TotalEarned)
				}
			}
		}
	}

	return result, nil
}

func memberDataFromMember(m Member) MemberData {
	tier := m.Tier
	if tier == "" {
		tier = CalculateTier(m.TotalEarned)
	}
	joinedAt := ""
	if !m.CreatedAt.IsZero() {
		joinedAt = m.CreatedAt.Format("2 Jan 2006")
	}
	lastActive := ""
	if !m.UpdatedAt.IsZero() {
		lastActive = m.UpdatedAt.Format("2 Jan 2006")
	}
	return MemberData{
		ID:            m.LineUID,
		LineUID:       m.LineUID,
		DisplayName:   m.DisplayName,
		PictureURL:    m.PictureURL,
		ShopsVisited:  []string{},
		CurrentPoints: int(m.PointBalance),
		TotalEarned:   int(m.TotalEarned),
		Tier:          tier,
		JoinedAt:      joinedAt,
		LastActive:    lastActive,
	}
}

func normalizePage(page, pageSize, defaultSize, maxSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultSize
	}
	if pageSize > maxSize {
		pageSize = maxSize
	}
	return page, pageSize
}

func GetMembers(filter MemberFilter) (*MemberListResult, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize, 20, 100)
	memberFilter := bson.M{}
	if q := strings.TrimSpace(filter.Query); q != "" {
		pattern := regexp.QuoteMeta(q)
		memberFilter["$or"] = []bson.M{
			{"display_name": bson.M{"$regex": pattern, "$options": "i"}},
			{"line_uid": bson.M{"$regex": pattern, "$options": "i"}},
		}
	}
	if tier := strings.TrimSpace(filter.Tier); tier != "" && tier != "all" {
		memberFilter["tier"] = tier
	}
	if shopID := strings.TrimSpace(filter.ShopID); shopID != "" && shopID != "all" {
		shopValues := []string{shopID}
		if shop, _ := GetShopByID(shopID); shop != nil {
			shopValues = append(shopValues, shop.Name)
		}
		or := make([]bson.M, 0, len(shopValues)*2)
		for _, v := range shopValues {
			or = append(or, bson.M{"shop_id": v}, bson.M{"shop_name": v})
		}
		lineUIDsRaw, err := s.pointTransColl.Distinct(ctx, "line_uid", bson.M{"$or": or})
		if err != nil {
			return nil, err
		}
		lineUIDs := make([]string, 0, len(lineUIDsRaw))
		for _, v := range lineUIDsRaw {
			if uid, ok := v.(string); ok && uid != "" {
				lineUIDs = append(lineUIDs, uid)
			}
		}
		if len(lineUIDs) == 0 {
			return &MemberListResult{Members: []MemberData{}, Total: 0, Page: filter.Page, PageSize: filter.PageSize}, nil
		}
		memberFilter["line_uid"] = bson.M{"$in": lineUIDs}
	}

	total, err := s.membersColl.CountDocuments(ctx, memberFilter)
	if err != nil {
		return nil, err
	}

	totalPoints := 0
	pointsPipeline := mongo.Pipeline{
		{{Key: "$match", Value: memberFilter}},
		{{Key: "$group", Value: bson.M{"_id": nil, "total": bson.M{"$sum": "$point_balance"}}}},
	}
	if cursor, err := s.membersColl.Aggregate(ctx, pointsPipeline); err == nil {
		defer cursor.Close(ctx)
		if cursor.Next(ctx) {
			var row struct {
				Total float64 `bson:"total"`
			}
			if cursor.Decode(&row) == nil {
				totalPoints = int(row.Total)
			}
		}
	}

	sort := bson.D{{Key: "updated_at", Value: -1}}
	switch filter.SortBy {
	case "points":
		sort = bson.D{{Key: "point_balance", Value: -1}}
	case "name":
		sort = bson.D{{Key: "display_name", Value: 1}}
	}

	findOpts := options.Find().SetSort(sort).SetSkip(int64((filter.Page - 1) * filter.PageSize)).SetLimit(int64(filter.PageSize))
	cursor, err := s.membersColl.Find(ctx, memberFilter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var members []Member
	if err := cursor.All(ctx, &members); err != nil {
		return nil, err
	}

	result := make([]MemberData, 0, len(members))
	memberIndex := make(map[string]int)
	lineUIDs := make([]string, 0, len(members))
	for _, m := range members {
		md := memberDataFromMember(m)
		result = append(result, md)
		memberIndex[m.LineUID] = len(result) - 1
		lineUIDs = append(lineUIDs, m.LineUID)
	}

	if len(lineUIDs) > 0 {
		pipeline := []bson.M{
			{"$match": bson.M{"line_uid": bson.M{"$in": lineUIDs}}},
			{"$group": bson.M{"_id": "$line_uid", "shops": bson.M{"$addToSet": "$shop_name"}, "total_earned": bson.M{"$sum": "$get_point"}}},
		}
		if aggCursor, err := s.pointTransColl.Aggregate(ctx, pipeline); err == nil {
			defer aggCursor.Close(ctx)
			for aggCursor.Next(ctx) {
				var row struct {
					LineUID     string   `bson:"_id"`
					Shops       []string `bson:"shops"`
					TotalEarned float64  `bson:"total_earned"`
				}
				if aggCursor.Decode(&row) == nil {
					if idx, ok := memberIndex[row.LineUID]; ok {
						result[idx].ShopsVisited = row.Shops
						result[idx].TotalEarned = int(row.TotalEarned)
					}
				}
			}
		}
	}

	return &MemberListResult{Members: result, Total: int(total), Page: filter.Page, PageSize: filter.PageSize, TotalPoints: totalPoints}, nil
}

func GetMemberDetail(lineUID string, txPage, txPageSize int) (*MemberDetailData, int, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var member Member
	if err := s.membersColl.FindOne(ctx, bson.M{"line_uid": lineUID}).Decode(&member); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	md := memberDataFromMember(member)
	shops, _ := GetMemberShops(lineUID)
	for _, shop := range shops {
		if shop.ShopName != "" {
			md.ShopsVisited = append(md.ShopsVisited, shop.ShopName)
		}
	}
	txs, _, total, err := GetAllTransactions(TransactionFilter{LineUID: lineUID, Page: txPage, PageSize: txPageSize})
	if err != nil {
		return nil, 0, err
	}
	return &MemberDetailData{Member: md, Shops: shops, Transactions: txs}, total, nil
}

func GetShopByID(shopID string) (*ShopData, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shopsColl := s.client.Database("bcmember").Collection("shops")
	var shop ShopData
	if err := shopsColl.FindOne(ctx, bson.M{"_id": shopID}).Decode(&shop); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &shop, nil
}
func GetShopDetail(shopID string, txLimit int) (*ShopDetailData, error) {
	shop, err := GetShopByID(shopID)
	if err != nil {
		return nil, err
	}
	if shop == nil {
		return nil, nil
	}

	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if txLimit <= 0 {
		txLimit = 20
	}
	if txLimit > 100 {
		txLimit = 100
	}

	shopOr := []bson.M{{"shop_id": shop.ID}}
	if strings.TrimSpace(shop.Name) != "" {
		shopOr = append(shopOr, bson.M{"shop_name": shop.Name})
	}
	shopMatch := bson.M{"$or": shopOr}

	detail := &ShopDetailData{}
	memberRows := make([]ShopMemberSummary, 0)
	lineUIDs := make([]string, 0)
	seenLineUID := make(map[string]bool)

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: shopMatch}},
		{{Key: "$group", Value: bson.M{
			"_id":        "$line_uid",
			"get_point":  bson.M{"$sum": "$get_point"},
			"use_point":  bson.M{"$sum": "$use_point"},
			"tx_count":   bson.M{"$sum": 1},
			"last_visit": bson.M{"$max": "$created_at"},
		}}},
		{{Key: "$addFields", Value: bson.M{"point_balance": bson.M{"$subtract": []interface{}{"$get_point", "$use_point"}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "point_balance", Value: -1}, {Key: "get_point", Value: -1}}}},
	}
	if cursor, err := s.pointTransColl.Aggregate(ctx, pipeline); err == nil {
		defer cursor.Close(ctx)
		for cursor.Next(ctx) {
			var row struct {
				LineUID      string    `bson:"_id"`
				GetPoint     float64   `bson:"get_point"`
				UsePoint     float64   `bson:"use_point"`
				PointBalance float64   `bson:"point_balance"`
				TxCount      int       `bson:"tx_count"`
				LastVisit    time.Time `bson:"last_visit"`
			}
			if cursor.Decode(&row) != nil {
				continue
			}
			memberRows = append(memberRows, ShopMemberSummary{
				LineUID:          row.LineUID,
				PointsEarned:     row.GetPoint,
				PointsUsed:       row.UsePoint,
				PointBalance:     row.PointBalance,
				TransactionCount: row.TxCount,
				LastVisit:        row.LastVisit,
			})
			detail.Stats.PointsEarned += row.GetPoint
			detail.Stats.PointsUsed += row.UsePoint
			detail.Stats.TransactionCount += row.TxCount
			if row.LineUID != "" && !seenLineUID[row.LineUID] {
				seenLineUID[row.LineUID] = true
				lineUIDs = append(lineUIDs, row.LineUID)
			}
		}
	}
	detail.Stats.PointBalance = detail.Stats.PointsEarned - detail.Stats.PointsUsed
	detail.Stats.MemberCount = len(lineUIDs)

	memberMap := make(map[string]*Member)
	if len(lineUIDs) > 0 {
		if mCursor, err := s.membersColl.Find(ctx, bson.M{"line_uid": bson.M{"$in": lineUIDs}}); err == nil {
			defer mCursor.Close(ctx)
			for mCursor.Next(ctx) {
				var m Member
				if mCursor.Decode(&m) == nil {
					memberMap[m.LineUID] = &m
				}
			}
		}
	}
	for i := range memberRows {
		if m := memberMap[memberRows[i].LineUID]; m != nil {
			memberRows[i].DisplayName = m.DisplayName
			memberRows[i].PictureURL = m.PictureURL
		} else if len(memberRows[i].LineUID) > 4 {
			memberRows[i].DisplayName = "User " + memberRows[i].LineUID[len(memberRows[i].LineUID)-4:]
		}
	}
	detail.Members = memberRows
	if len(memberRows) > 20 {
		detail.TopMembers = memberRows[:20]
	} else {
		detail.TopMembers = memberRows
	}

	findOpts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(txLimit))
	if txCursor, err := s.pointTransColl.Find(ctx, shopMatch, findOpts); err == nil {
		defer txCursor.Close(ctx)
		for txCursor.Next(ctx) {
			var tx PointTransaction
			if txCursor.Decode(&tx) == nil {
				detail.RecentTransactions = append(detail.RecentTransactions, transactionDataFromPointTransaction(tx, memberMap))
			}
		}
	}

	issuesColl := s.client.Database("bcmember").Collection("invoice_issues")
	if issueCursor, err := issuesColl.Find(ctx, shopMatch, findOpts); err == nil {
		defer issueCursor.Close(ctx)
		for issueCursor.Next(ctx) {
			var issue invoiceIssueRecord
			if issueCursor.Decode(&issue) == nil {
				detail.InvoiceIssues = append(detail.InvoiceIssues, InvoiceIssueData{
					ShopID:    issue.ShopID,
					ShopName:  issue.ShopName,
					DocNo:     issue.DocNo,
					LineUID:   issue.LineUID,
					IssueType: issue.IssueType,
					Reason:    issue.Reason,
					CreatedAt: issue.CreatedAt.Format(time.RFC3339),
				})
			}
		}
	}

	return detail, nil
}
func IsShopActive(shop *ShopData) bool {
	if shop == nil {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(shop.Status))
	return status == "" || status == "active"
}

func RotateShopAPIKey(shopID string) (string, error) {
	newKey := "bc_live_" + generateRandomString(32)
	updates := map[string]interface{}{
		"api_key": newKey,
		"status":  "active",
	}
	if err := UpdateShop(shopID, updates); err != nil {
		return "", err
	}
	return newKey, nil
}

func RevokeShopAPIKey(shopID string) error {
	return UpdateShop(shopID, map[string]interface{}{
		"api_key": "",
		"status":  "inactive",
	})
}

func PreviewShopPoints(shopID string, amount, redeemPoint float64) (*PointRulePreview, error) {
	shop, err := GetShopByID(shopID)
	if err != nil {
		return nil, err
	}
	if shop == nil {
		return nil, nil
	}
	if shop.RedeemRate <= 0 {
		shop.RedeemRate = 1
	}
	return &PointRulePreview{
		Amount:      amount,
		GetPoint:    CalculatePoints(shop, amount),
		RedeemPoint: redeemPoint,
		RedeemValue: redeemPoint * float64(shop.RedeemRate),
	}, nil
}
func isAdminAdjustment(tx PointTransaction) bool {
	return tx.Source == "admin_adjust" || tx.ShopID == "admin" || strings.HasPrefix(tx.DocNo, "ADJ_")
}

func transactionDataFromPointTransaction(tx PointTransaction, memberMap map[string]*Member) TransactionData {
	txType := "earn"
	points := int(tx.GetPoint)
	if isAdminAdjustment(tx) {
		txType = "adjust"
		points = int(tx.GetPoint - tx.UsePoint)
	} else if tx.UsePoint > 0 {
		txType = "redeem"
		points = -int(tx.UsePoint)
	}

	memberName := "สมาชิก"
	memberPicture := ""
	if m, ok := memberMap[tx.LineUID]; ok && m != nil {
		if m.DisplayName != "" {
			memberName = m.DisplayName
		}
		memberPicture = m.PictureURL
	} else if len(tx.LineUID) > 4 {
		memberName = "ผู้ใช้ " + tx.LineUID[len(tx.LineUID)-4:]
	}

	return TransactionData{
		ID:            tx.DocNo,
		TransactionID: tx.DocNo,
		MemberUID:     tx.LineUID,
		MemberName:    memberName,
		MemberPicture: memberPicture,
		ShopID:        tx.ShopID,
		Shop:          tx.ShopName,
		Type:          txType,
		Points:        points,
		GetPoint:      tx.GetPoint,
		UsePoint:      tx.UsePoint,
		Amount:        0,
		Date:          tx.CreatedAt.Format(time.RFC3339),
		Note:          tx.Note,
		Source:        tx.Source,
		BalanceBefore: tx.BalanceBefore,
		BalanceAfter:  tx.BalanceAfter,
	}
}

// AdjustMemberPoints adjusts a member's points by admin (add or deduct)
// It creates a transaction record with reason/evidence and updates the member balance.
func AdjustMemberPoints(lineUID, adjustType string, points int, note string) (*AdjustPointResult, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	lineUID = strings.TrimSpace(lineUID)
	note = strings.TrimSpace(note)
	if lineUID == "" {
		return nil, fmt.Errorf("line_uid is required")
	}
	if points <= 0 {
		return nil, fmt.Errorf("points must be greater than 0")
	}
	if adjustType != "add" && adjustType != "deduct" {
		return nil, fmt.Errorf("type must be add or deduct")
	}
	if note == "" {
		return nil, fmt.Errorf("reason is required")
	}

	membersColl := s.membersColl
	var current Member
	previousBalance := float64(0)
	if err := membersColl.FindOne(ctx, bson.M{"line_uid": lineUID}).Decode(&current); err == nil {
		previousBalance = current.PointBalance
	} else if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("failed to get member: %v", err)
	}

	var getPoint, usePoint float64
	if adjustType == "add" {
		getPoint = float64(points)
	} else {
		usePoint = float64(points)
		if usePoint > previousBalance {
			return nil, fmt.Errorf("insufficient points balance")
		}
	}

	pointChange := getPoint - usePoint
	newBalanceExpected := previousBalance + pointChange
	docNo := fmt.Sprintf("ADJ_%d", time.Now().UnixNano())
	trans := PointTransaction{
		LineUID:       lineUID,
		ShopID:        "admin",
		ShopName:      "Admin Adjustment",
		DocNo:         docNo,
		GetPoint:      getPoint,
		UsePoint:      usePoint,
		Note:          note,
		Source:        "admin_adjust",
		BalanceBefore: previousBalance,
		BalanceAfter:  newBalanceExpected,
		CreatedAt:     time.Now(),
	}
	if _, err := s.pointTransColl.InsertOne(ctx, trans); err != nil {
		return nil, fmt.Errorf("failed to save transaction: %v", err)
	}

	incFields := bson.M{"point_balance": pointChange}
	if getPoint > 0 {
		incFields["total_earned"] = getPoint
	}
	filter := bson.M{"line_uid": lineUID}
	update := bson.M{
		"$inc":         incFields,
		"$set":         bson.M{"updated_at": time.Now()},
		"$setOnInsert": bson.M{"line_uid": lineUID, "created_at": time.Now()},
	}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After).SetUpsert(true)

	var member Member
	if err := membersColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&member); err != nil {
		return nil, fmt.Errorf("failed to update member points: %v", err)
	}

	newTier := CalculateTier(member.TotalEarned)
	if member.Tier != newTier {
		_, _ = membersColl.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"tier": newTier}})
	}

	return &AdjustPointResult{NewBalance: member.PointBalance, PreviousBalance: previousBalance, TransactionID: docNo}, nil
}

// TransactionFilter for server-side filtering and pagination
type TransactionFilter struct {
	StartDate string // "2025-01-15" format
	EndDate   string // "2025-01-20" format
	ShopID    string
	LineUID   string
	Query     string
	TxType    string // "earn" | "redeem" | "adjust" | "" for all
	Page      int
	PageSize  int
}

// GetAllTransactions returns filtered transactions for admin view with server-side pagination
func GetAllTransactions(filter TransactionFilter) ([]TransactionData, *TodayStatsData, int, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, nil, 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filter.Page, filter.PageSize = normalizePage(filter.Page, filter.PageSize, 50, 200)
	mongoFilter := bson.M{}

	if filter.StartDate != "" {
		if startTime, err := time.Parse("2006-01-02", filter.StartDate); err == nil {
			mongoFilter["created_at"] = bson.M{"$gte": startTime}
		}
	}
	if filter.EndDate != "" {
		if endTime, err := time.Parse("2006-01-02", filter.EndDate); err == nil {
			endTime = endTime.Add(24*time.Hour - time.Second)
			if mongoFilter["created_at"] == nil {
				mongoFilter["created_at"] = bson.M{}
			}
			mongoFilter["created_at"].(bson.M)["$lte"] = endTime
		}
	}
	if shopID := strings.TrimSpace(filter.ShopID); shopID != "" && shopID != "all" {
		mongoFilter["shop_id"] = shopID
	}
	if lineUID := strings.TrimSpace(filter.LineUID); lineUID != "" {
		mongoFilter["line_uid"] = lineUID
	}
	if q := strings.TrimSpace(filter.Query); q != "" {
		pattern := regexp.QuoteMeta(q)
		mongoFilter["$or"] = []bson.M{
			{"doc_no": bson.M{"$regex": pattern, "$options": "i"}},
			{"line_uid": bson.M{"$regex": pattern, "$options": "i"}},
			{"shop_id": bson.M{"$regex": pattern, "$options": "i"}},
			{"shop_name": bson.M{"$regex": pattern, "$options": "i"}},
			{"note": bson.M{"$regex": pattern, "$options": "i"}},
		}
	}

	switch filter.TxType {
	case "earn":
		mongoFilter["get_point"] = bson.M{"$gt": 0}
		mongoFilter["source"] = bson.M{"$ne": "admin_adjust"}
		if _, hasShopFilter := mongoFilter["shop_id"]; !hasShopFilter {
			mongoFilter["shop_id"] = bson.M{"$ne": "admin"}
		}
	case "redeem":
		mongoFilter["use_point"] = bson.M{"$gt": 0}
		mongoFilter["source"] = bson.M{"$ne": "admin_adjust"}
		if _, hasShopFilter := mongoFilter["shop_id"]; !hasShopFilter {
			mongoFilter["shop_id"] = bson.M{"$ne": "admin"}
		}
	case "adjust":
		mongoFilter["$or_adjust"] = []bson.M{
			{"source": "admin_adjust"},
			{"shop_id": "admin"},
			{"doc_no": bson.M{"$regex": "^ADJ_"}},
		}
	}
	if adjustOr, ok := mongoFilter["$or_adjust"]; ok {
		delete(mongoFilter, "$or_adjust")
		if existingOr, hasExisting := mongoFilter["$or"]; hasExisting {
			mongoFilter["$and"] = []bson.M{{"$or": existingOr}, {"$or": adjustOr}}
			delete(mongoFilter, "$or")
		} else {
			mongoFilter["$or"] = adjustOr
		}
	}

	total, err := s.pointTransColl.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, nil, 0, err
	}

	findOpts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((filter.Page - 1) * filter.PageSize)).
		SetLimit(int64(filter.PageSize))

	cursor, err := s.pointTransColl.Find(ctx, mongoFilter, findOpts)
	if err != nil {
		return nil, nil, 0, err
	}
	defer cursor.Close(ctx)

	var transactions []PointTransaction
	if err := cursor.All(ctx, &transactions); err != nil {
		return nil, nil, 0, err
	}

	memberMap := make(map[string]*Member)
	lineUIDs := make([]string, 0)
	for _, tx := range transactions {
		if tx.LineUID == "" {
			continue
		}
		if _, exists := memberMap[tx.LineUID]; !exists {
			memberMap[tx.LineUID] = nil
			lineUIDs = append(lineUIDs, tx.LineUID)
		}
	}
	if len(lineUIDs) > 0 {
		mCursor, mErr := s.membersColl.Find(ctx, bson.M{"line_uid": bson.M{"$in": lineUIDs}})
		if mErr == nil {
			defer mCursor.Close(ctx)
			for mCursor.Next(ctx) {
				var m Member
				if mCursor.Decode(&m) == nil {
					memberMap[m.LineUID] = &m
				}
			}
		}
	}

	result := make([]TransactionData, 0, len(transactions))
	for _, tx := range transactions {
		result = append(result, transactionDataFromPointTransaction(tx, memberMap))
	}

	todayStats := &TodayStatsData{}
	today := time.Now().Truncate(24 * time.Hour)
	todayCursor, todayErr := s.pointTransColl.Find(ctx, bson.M{"created_at": bson.M{"$gte": today}})
	if todayErr == nil {
		defer todayCursor.Close(ctx)
		for todayCursor.Next(ctx) {
			var tx PointTransaction
			if todayCursor.Decode(&tx) == nil {
				todayStats.Count++
				todayStats.Earned += int(tx.GetPoint)
				todayStats.Used += int(tx.UsePoint)
			}
		}
	}
	todayStats.Net = todayStats.Earned - todayStats.Used

	return result, todayStats, int(total), nil
}

// ========== Chart Data ==========

// DailyPointsData represents daily points aggregation
type DailyPointsData struct {
	Date         string `json:"date"`
	PointsEarned int    `json:"points_earned"`
	PointsUsed   int    `json:"points_used"`
}

// DailyMembersData represents daily new member count
type DailyMembersData struct {
	Date     string `json:"date"`
	NewCount int    `json:"new_count"`
}

// ChartData holds chart data for dashboard
type ChartData struct {
	DailyPoints  []DailyPointsData  `json:"daily_points"`
	DailyMembers []DailyMembersData `json:"daily_members"`
}

// GetDashboardChartData returns aggregated daily data for charts
func GetDashboardChartData(days int) (*ChartData, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	startDate := time.Now().Truncate(24*time.Hour).AddDate(0, 0, -(days - 1))

	// Points aggregation by date
	pointsPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"created_at": bson.M{"$gte": startDate},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"$dateToString": bson.M{
					"format": "%Y-%m-%d",
					"date":   "$created_at",
				},
			},
			"points_earned": bson.M{"$sum": "$get_point"},
			"points_used":   bson.M{"$sum": "$use_point"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}

	pointsMap := make(map[string]DailyPointsData)
	aggCursor, err := s.pointTransColl.Aggregate(ctx, pointsPipeline)
	if err == nil {
		defer aggCursor.Close(ctx)
		for aggCursor.Next(ctx) {
			var row struct {
				Date         string  `bson:"_id"`
				PointsEarned float64 `bson:"points_earned"`
				PointsUsed   float64 `bson:"points_used"`
			}
			if aggCursor.Decode(&row) == nil {
				pointsMap[row.Date] = DailyPointsData{
					Date:         row.Date,
					PointsEarned: int(row.PointsEarned),
					PointsUsed:   int(row.PointsUsed),
				}
			}
		}
	}

	// Members aggregation by created_at date
	membersColl := s.membersColl
	membersPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"created_at": bson.M{"$gte": startDate},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"$dateToString": bson.M{
					"format": "%Y-%m-%d",
					"date":   "$created_at",
				},
			},
			"new_count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}

	membersMap := make(map[string]DailyMembersData)
	memCursor, err := membersColl.Aggregate(ctx, membersPipeline)
	if err == nil {
		defer memCursor.Close(ctx)
		for memCursor.Next(ctx) {
			var row struct {
				Date     string `bson:"_id"`
				NewCount int    `bson:"new_count"`
			}
			if memCursor.Decode(&row) == nil {
				membersMap[row.Date] = DailyMembersData{
					Date:     row.Date,
					NewCount: row.NewCount,
				}
			}
		}
	}

	// Fill in all dates with zero defaults
	chartData := &ChartData{
		DailyPoints:  make([]DailyPointsData, 0, days),
		DailyMembers: make([]DailyMembersData, 0, days),
	}
	for d := 0; d < days; d++ {
		dateStr := startDate.AddDate(0, 0, d).Format("2006-01-02")
		if pd, ok := pointsMap[dateStr]; ok {
			chartData.DailyPoints = append(chartData.DailyPoints, pd)
		} else {
			chartData.DailyPoints = append(chartData.DailyPoints, DailyPointsData{Date: dateStr})
		}
		if md, ok := membersMap[dateStr]; ok {
			chartData.DailyMembers = append(chartData.DailyMembers, md)
		} else {
			chartData.DailyMembers = append(chartData.DailyMembers, DailyMembersData{Date: dateStr})
		}
	}

	return chartData, nil
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

// ========== Audit Logs ==========

// SaveAuditLog records an admin action to the audit_logs collection
func SaveAuditLog(adminLineUID, adminName, action, targetType, targetID, details string) {
	s, err := getOrCreateStore()
	if err != nil {
		fmt.Printf("ERROR: SaveAuditLog store: %v\n", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	coll := s.client.Database("bcmember").Collection("audit_logs")
	_, err = coll.InsertOne(ctx, AuditLog{
		AdminLineUID: adminLineUID,
		AdminName:    adminName,
		Action:       action,
		TargetType:   targetType,
		TargetID:     targetID,
		Details:      details,
		CreatedAt:    time.Now(),
	})
	if err != nil {
		fmt.Printf("ERROR: SaveAuditLog insert: %v\n", err)
	}
}

// GetAuditLogs retrieves audit logs with filtering and pagination
func GetAuditLogs(filter AuditLogFilter) ([]AuditLog, int, error) {
	s, err := getOrCreateStore()
	if err != nil {
		return nil, 0, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	coll := s.client.Database("bcmember").Collection("audit_logs")

	// Build dynamic filter
	query := bson.M{}
	if filter.Action != "" {
		query["action"] = filter.Action
	}
	if filter.StartDate != "" {
		if t, err := time.Parse("2006-01-02", filter.StartDate); err == nil {
			if query["created_at"] == nil {
				query["created_at"] = bson.M{}
			}
			query["created_at"].(bson.M)["$gte"] = t
		}
	}
	if filter.EndDate != "" {
		if t, err := time.Parse("2006-01-02", filter.EndDate); err == nil {
			endOfDay := t.Add(24*time.Hour - time.Millisecond)
			if query["created_at"] == nil {
				query["created_at"] = bson.M{}
			}
			query["created_at"].(bson.M)["$lte"] = endOfDay
		}
	}

	// Count total
	total, err := coll.CountDocuments(ctx, query)
	if err != nil {
		return nil, 0, err
	}

	// Pagination defaults
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	skip := (filter.Page - 1) * filter.PageSize

	// Query with sort and pagination
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64(skip)).
		SetLimit(int64(filter.PageSize))

	cursor, err := coll.Find(ctx, query, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var logs []AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, 0, err
	}

	return logs, int(total), nil
}
