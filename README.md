# BCMember API - LINE OA Login & AI Chatbot

Backend API สำหรับระบบ Login ผ่าน LINE OA พร้อม AI Chatbot โดยใช้ Gemini AI

## 🚀 Features

- ✅ **LINE OA Login** - Login ผ่านรหัส 4 หลัก
- ✅ **AI Chatbot** - ตอบคำถามด้วย Gemini AI พร้อมเก็บประวัติการสนทนา
- ✅ **Multi-Shop Support** - รองรับหลายร้านค้าในระบบเดียว
- ✅ **MongoDB Atlas** - เก็บข้อมูลบน Cloud
- ✅ **Token Optimization** - ประหยัด AI token ด้วยการจำกัดประวัติการสนทนา

## 📋 Table of Contents

- [Quick Start](#quick-start)
- [API Documentation](#api-documentation)
- [Frontend Integration](#frontend-integration)
- [Deployment](#deployment)
- [Environment Variables](#environment-variables)

## ⚡ Quick Start

### Prerequisites

- Go 1.21+
- MongoDB Atlas account
- LINE OA account
- Gemini API key

### Installation

```bash
# Clone repository
git clone https://github.com/your-username/bcmemberapi.git
cd bcmemberapi

# Install dependencies
go mod download

# Copy .env.example to .env
cp .env.example .env

# Edit .env and add your credentials
nano .env

# Run
go run main.go
```

Server จะรันที่ `http://localhost:8080`

## 📚 API Documentation

### 1. Generate Login Code

สร้างรหัส 4 หลักสำหรับ Login

```http
POST /api/login/code
Content-Type: application/json

{
  "shop_id": "shop_123"
}
```

**Response:**
```json
{
  "code": "1234"
}
```

### 2. Check Login Status

ตรวจสอบสถานะการ Login (polling ทุก 1 วินาที)

```http
GET /api/login/status?code=1234
```

**Response (Pending):**
```json
{
  "code": "1234",
  "shop_id": "shop_123",
  "status": "pending",
  "created_at": "2025-12-17T10:00:00Z",
  "expires_at": "2025-12-17T10:05:00Z"
}
```

**Response (Success):**
```json
{
  "code": "1234",
  "shop_id": "shop_123",
  "status": "success",
  "line_user_id": "U1234567890abcdef",
  "display_name": "สมชาย ใจดี",
  "created_at": "2025-12-17T10:00:00Z",
  "expires_at": "2025-12-17T10:05:00Z"
}
```

### 3. LINE Webhook

รับข้อความจาก LINE OA

```http
POST /callback
```

**Behavior:**
- ถ้าข้อความเป็นตัวเลข 4 หลัก → ตรวจสอบเป็นรหัส Login
- ถ้าไม่ใช่ → ส่งไปให้ AI ตอบ พร้อมเก็บประวัติการสนทนา

## 🎨 Frontend Integration

### React/Web
ดูเอกสารเต็มที่ [FRONTEND_INTEGRATION.md](FRONTEND_INTEGRATION.md)

### Flutter
ดูเอกสารเต็มที่ [FLUTTER_INTEGRATION.md](FLUTTER_INTEGRATION.md)

## 🚀 Deployment

### แนะนำ: Railway.app

```bash
# 1. Push to GitHub
git init
git add .
git commit -m "Initial commit"
git push origin main

# 2. Deploy to Railway
# - ไปที่ https://railway.app
# - Deploy from GitHub repo
# - ตั้งค่า Environment Variables
# - Deploy!
```

ดูเอกสารเต็มที่ [DEPLOYMENT.md](DEPLOYMENT.md)

**Alternatives:**
- ✅ Render.com
- ✅ Fly.io
- ❌ Vercel (ไม่แนะนำสำหรับ Go HTTP Server)

## 🔧 Environment Variables

```bash
# LINE OA
LINE_CHANNEL_SECRET=your_line_channel_secret
LINE_CHANNEL_TOKEN=your_line_channel_token

# Gemini AI
GEMINI_API_KEY=your_gemini_api_key

# MongoDB Atlas
MONGODB_URI=mongodb+srv://username:password@cluster.mongodb.net/dbname?retryWrites=true&w=majority
MONGODB_DBNAME=bcmember

# Server
PORT=8080
```

## 📦 Project Structure

```
bcmemberapi/
├── main.go                    # Main application
├── ai/
│   └── gemini.go             # Gemini AI service
├── store/
│   └── mongo.go              # MongoDB operations
├── .env                       # Environment variables
├── go.mod                     # Go dependencies
├── Procfile                   # For Railway deployment
├── railway.toml               # Railway configuration
├── Dockerfile                 # Docker configuration
├── README.md                  # This file
├── DEPLOYMENT.md              # Deployment guide
├── FRONTEND_INTEGRATION.md    # Web frontend guide
└── FLUTTER_INTEGRATION.md     # Flutter guide
```

## 🗄️ MongoDB Collections

### 1. login_codes
```json
{
  "code": "1234",
  "shop_id": "shop_123",
  "status": "pending",
  "line_user_id": "U1234...",
  "display_name": "สมชาย ใจดี",
  "created_at": "2025-12-17T10:00:00Z",
  "expires_at": "2025-12-17T10:05:00Z"
}
```

### 2. chat_history
```json
{
  "user_id": "U1234...",
  "role": "user",
  "message": "สวัสดีครับ",
  "created_at": "2025-12-17T10:00:00Z"
}
```

## 🎯 User Flow

```
1. Frontend เรียก POST /api/login/code → ได้รหัส 4 หลัก
   ↓
2. Frontend แสดงรหัสให้ผู้ใช้เห็น
   ↓
3. ผู้ใช้เปิด LINE OA @bcmember พิมพ์รหัส
   ↓
4. Backend รับรหัส → ตรวจสอบ → อัปเดตสถานะ
   ↓
5. Frontend polling GET /api/login/status ทุก 1 วินาที
   ↓
6. เมื่อ status = "success" → ได้ LINE ID และชื่อผู้ใช้
   ↓
7. Frontend redirect ไปหน้าหลัก
```

## 🔒 Security

- ✅ Input validation
- ✅ Code expiration (5 นาที)
- ✅ LINE signature verification
- ✅ MongoDB Atlas encryption
- ✅ HTTPS only (production)
- ✅ Environment variables for secrets

## 🎨 AI Features

- ✅ Gemini 1.5 Flash (เร็ว + ถูก)
- ✅ เก็บประวัติการสนทนา 6 ข้อความ (ประหยัด token 40%)
- ✅ Context-aware responses
- ✅ Error handling
- ✅ Fallback messages

## 📊 Performance

- ⚡ Response time: <100ms (average)
- 💾 MongoDB: Optimized queries with indexes
- 🔄 Polling: 1 second interval
- 💰 AI Token: ประหยัด 40% (6 vs 10 messages)

## 🐛 Troubleshooting

### MongoDB Connection Error
```bash
# ตรวจสอบ:
1. MONGODB_URI ถูกต้องหรือไม่
2. Network Access whitelist 0.0.0.0/0
3. Database name ถูกต้อง
```

### LINE Webhook ไม่ทำงาน
```bash
# ตรวจสอบ:
1. Webhook URL ตั้งค่าใน LINE Developers Console
2. SSL certificate valid (https://)
3. Verify webhook enable
```

### AI ไม่ตอบ
```bash
# ตรวจสอบ:
1. GEMINI_API_KEY ถูกต้อง
2. Quota ยังเหลืออยู่
3. ดู logs มี error หรือไม่
```

## 📝 License

MIT License - สามารถนำไปใช้ได้ฟรี

## 👥 Contributing

Pull requests are welcome!

## 📧 Contact

- LINE OA: @bcmember
- Email: jaturapornchai@gmail.com

---

**Made with ❤️ and ☕**
