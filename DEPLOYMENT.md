# Deployment Guide - BCMember API

## ⚠️ สำคัญ: เกี่ยวกับ Vercel

**Vercel ไม่เหมาะกับ Go HTTP Server แบบนี้** เพราะ:
- ❌ Vercel ออกแบบมาสำหรับ Serverless Functions
- ❌ ไม่รองรับ persistent HTTP server
- ❌ Function timeout จำกัด (ฟรี: 10s, Pro: 60s)
- ❌ ไม่รองรับ WebSocket/Long polling ได้ดี
- ❌ MongoDB connection pooling ไม่ทำงานได้ดี

**✅ แนะนำ Alternatives ที่เหมาะกว่า:**
1. **Railway.app** - ฟรี $5/เดือน credit, รองรับ Go เต็มรูปแบบ
2. **Render.com** - มี free tier, deploy ง่าย
3. **Fly.io** - ฟรี 3 VMs, เร็วมาก
4. **Google Cloud Run** - Pay-as-you-go, scale to zero

---

## 📋 สารบัญ
1. [Railway.app (แนะนำ)](#1-railwayapp-แนะนำ)
2. [Render.com](#2-rendercom)
3. [Fly.io](#3-flyio)
4. [Vercel (Serverless)](#4-vercel-serverless)
5. [Docker Deployment](#5-docker-deployment)

---

## 1. Railway.app (แนะนำ)

### ทำไมต้อง Railway?
- ✅ รองรับ Go HTTP Server เต็มรูปแบบ
- ✅ มี free credit $5/เดือน
- ✅ Deploy จาก GitHub ได้
- ✅ มี built-in domain & SSL
- ✅ Environment variables ง่าย
- ✅ MongoDB connection ทำงานได้ดี

### ขั้นตอน Deploy

#### 1. สร้างไฟล์ Procfile
```bash
# Procfile (ไม่มี extension)
web: ./bcmemberapi
```

#### 2. สร้าง railway.toml
```toml
[build]
builder = "nixpacks"
buildCommand = "go build -o bcmemberapi"

[deploy]
startCommand = "./bcmemberapi"
healthcheckPath = "/"
healthcheckTimeout = 100
restartPolicyType = "on_failure"
restartPolicyMaxRetries = 10
```

#### 3. Push to GitHub
```bash
git init
git add .
git commit -m "Initial commit"
git branch -M main
git remote add origin https://github.com/YOUR_USERNAME/bcmemberapi.git
git push -u origin main
```

#### 4. Deploy to Railway

1. ไปที่ [railway.app](https://railway.app)
2. Sign up ด้วย GitHub
3. คลิก "New Project"
4. เลือก "Deploy from GitHub repo"
5. เลือก repository `bcmemberapi`
6. Railway จะ auto-detect Go project

#### 5. ตั้งค่า Environment Variables

ใน Railway Dashboard → Variables:
```bash
PORT=8080
LINE_CHANNEL_SECRET=your_secret
LINE_CHANNEL_TOKEN=your_token
GEMINI_API_KEY=your_api_key
MONGODB_URI=your_mongodb_uri
MONGODB_DBNAME=bcmember
```

#### 6. ตั้งค่า Custom Domain (Optional)

1. Railway → Settings → Domains
2. คลิก "Generate Domain" หรือเพิ่ม custom domain
3. Railway จะให้ URL เช่น: `bcmemberapi-production.up.railway.app`

#### 7. Deploy

Railway จะ auto-deploy ทุกครั้งที่ push to GitHub

**ราคา:**
- Free tier: $5 credit/เดือน (พอสำหรับ development)
- Hobby: $5/เดือน (แนะนำสำหรับ production)

---

## 2. Render.com

### ทำไมต้อง Render?
- ✅ มี Free tier
- ✅ Deploy ง่าย
- ✅ Auto SSL
- ✅ Zero-config deployment

### ขั้นตอน Deploy

#### 1. สร้างไฟล์ render.yaml
```yaml
services:
  - type: web
    name: bcmemberapi
    env: go
    buildCommand: go build -o bcmemberapi
    startCommand: ./bcmemberapi
    envVars:
      - key: PORT
        value: 8080
      - key: LINE_CHANNEL_SECRET
        sync: false
      - key: LINE_CHANNEL_TOKEN
        sync: false
      - key: GEMINI_API_KEY
        sync: false
      - key: MONGODB_URI
        sync: false
      - key: MONGODB_DBNAME
        value: bcmember
```

#### 2. Push to GitHub

#### 3. Deploy to Render

1. ไปที่ [render.com](https://render.com)
2. Sign up ด้วย GitHub
3. คลิก "New +" → "Web Service"
4. เชื่อมต่อ GitHub repository
5. ตั้งค่า:
   - **Name**: bcmemberapi
   - **Environment**: Go
   - **Build Command**: `go build -o bcmemberapi`
   - **Start Command**: `./bcmemberapi`

#### 4. ตั้งค่า Environment Variables

ใน Render Dashboard → Environment:
```bash
PORT=8080
LINE_CHANNEL_SECRET=your_secret
LINE_CHANNEL_TOKEN=your_token
GEMINI_API_KEY=your_api_key
MONGODB_URI=your_mongodb_uri
MONGODB_DBNAME=bcmember
```

#### 5. Deploy

Render จะ auto-deploy

**ราคา:**
- Free tier: มีข้อจำกัด (spin down after 15 mins of inactivity)
- Starter: $7/เดือน (แนะนำ)

---

## 3. Fly.io

### ทำไมต้อง Fly.io?
- ✅ Free tier ดี (3 shared VMs)
- ✅ Deploy เร็วมาก
- ✅ Global edge network
- ✅ Scale ได้ดี

### ขั้นตอน Deploy

#### 1. Install Fly CLI
```bash
# Windows (PowerShell)
powershell -Command "iwr https://fly.io/install.ps1 -useb | iex"

# Mac/Linux
curl -L https://fly.io/install.sh | sh
```

#### 2. Login
```bash
fly auth login
```

#### 3. สร้าง Dockerfile
```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o bcmemberapi

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/bcmemberapi .

# Expose port
EXPOSE 8080

# Run
CMD ["./bcmemberapi"]
```

#### 4. Initialize Fly App
```bash
fly launch
```

Fly จะถาม:
- App name: `bcmemberapi`
- Region: เลือกใกล้ที่สุด
- Postgres: `No` (เราใช้ MongoDB Atlas)
- Redis: `No`

#### 5. ตั้งค่า Environment Variables
```bash
fly secrets set \
  PORT=8080 \
  LINE_CHANNEL_SECRET=your_secret \
  LINE_CHANNEL_TOKEN=your_token \
  GEMINI_API_KEY=your_api_key \
  MONGODB_URI=your_mongodb_uri \
  MONGODB_DBNAME=bcmember
```

#### 6. Deploy
```bash
fly deploy
```

#### 7. เปิด App
```bash
fly open
```

**ราคา:**
- Free tier: 3 shared VMs (256MB RAM each)
- Hobby: ~$2-5/เดือน

---

## 4. Vercel (Serverless)

⚠️ **ต้องปรับโครงสร้างเป็น Serverless Functions**

### โครงสร้างใหม่

```
api/
├── login/
│   ├── code.go       # POST /api/login/code
│   └── status.go     # GET /api/login/status
└── callback.go       # POST /api/callback
```

### ตัวอย่าง Serverless Function

**api/login/code.go**
```go
package login

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/your-username/bcmemberapi/store"
)

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	// Parse request
	var req struct {
		ShopID string `json:"shop_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Initialize MongoDB
	mongoURI := os.Getenv("MONGODB_URI")
	st, err := store.NewStore(mongoURI)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate code
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	code, err := st.GenerateCode(ctx, req.ShopID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Response
	json.NewEncoder(w).Encode(map[string]string{"code": code})
}
```

### vercel.json
```json
{
  "version": 2,
  "builds": [
    {
      "src": "api/**/*.go",
      "use": "@vercel/go"
    }
  ],
  "routes": [
    {
      "src": "/api/login/code",
      "dest": "/api/login/code.go"
    },
    {
      "src": "/api/login/status",
      "dest": "/api/login/status.go"
    },
    {
      "src": "/api/callback",
      "dest": "/api/callback.go"
    }
  ],
  "env": {
    "PORT": "8080"
  }
}
```

### Deploy to Vercel

```bash
# Install Vercel CLI
npm i -g vercel

# Login
vercel login

# Deploy
vercel
```

**ข้อจำกัด:**
- ❌ Function timeout: 10s (free), 60s (pro)
- ❌ ไม่มี persistent connections
- ❌ Cold start ช้า
- ❌ แก้โค้ดเยอะ

---

## 5. Docker Deployment

สำหรับ deploy ที่ไหนก็ได้ที่รองรับ Docker

### Dockerfile (Production-ready)

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o bcmemberapi

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary
COPY --from=builder /app/bcmemberapi .

# Copy .env (optional, better to use environment variables)
# COPY .env .

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ || exit 1

# Run
CMD ["./bcmemberapi"]
```

### docker-compose.yml (For local testing)

```yaml
version: '3.8'

services:
  api:
    build: .
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - LINE_CHANNEL_SECRET=${LINE_CHANNEL_SECRET}
      - LINE_CHANNEL_TOKEN=${LINE_CHANNEL_TOKEN}
      - GEMINI_API_KEY=${GEMINI_API_KEY}
      - MONGODB_URI=${MONGODB_URI}
      - MONGODB_DBNAME=bcmember
    restart: unless-stopped
```

### Build & Run

```bash
# Build
docker build -t bcmemberapi .

# Run
docker run -p 8080:8080 \
  -e PORT=8080 \
  -e LINE_CHANNEL_SECRET=your_secret \
  -e LINE_CHANNEL_TOKEN=your_token \
  -e GEMINI_API_KEY=your_api_key \
  -e MONGODB_URI=your_mongodb_uri \
  bcmemberapi
```

---

## 📊 Comparison Table

| Platform | Free Tier | Deploy Speed | Difficulty | Best For |
|----------|-----------|--------------|------------|----------|
| **Railway** | $5 credit | ⚡⚡⚡ Fast | ⭐ Easy | **แนะนำ** |
| **Render** | Yes (limited) | ⚡⚡ Medium | ⭐ Easy | Development |
| **Fly.io** | 3 VMs | ⚡⚡⚡ Fast | ⭐⭐ Medium | Global apps |
| **Vercel** | Yes | ⚡⚡ Medium | ⭐⭐⭐ Hard | ❌ ไม่แนะนำ |

---

## 🚀 Quick Start (Railway - แนะนำ)

```bash
# 1. Push to GitHub
git init
git add .
git commit -m "Initial commit"
git push origin main

# 2. ไปที่ railway.app
# 3. Deploy from GitHub repo
# 4. ตั้งค่า Environment Variables
# 5. Deploy!
```

---

## ✅ Pre-Deployment Checklist

### Code
- [ ] Remove debug logs
- [ ] Update MongoDB connection with production URI
- [ ] Set proper CORS headers
- [ ] Add rate limiting
- [ ] Error handling ครบถ้วน

### Environment Variables
- [ ] LINE_CHANNEL_SECRET
- [ ] LINE_CHANNEL_TOKEN
- [ ] GEMINI_API_KEY
- [ ] MONGODB_URI (MongoDB Atlas)
- [ ] PORT (default: 8080)

### Security
- [ ] ใช้ HTTPS only
- [ ] Hide sensitive data
- [ ] Input validation
- [ ] Rate limiting

### MongoDB
- [ ] ใช้ MongoDB Atlas
- [ ] Whitelist IP addresses
- [ ] สร้าง index สำหรับ fields ที่ query บ่อย

### LINE OA
- [ ] ตั้งค่า Webhook URL ใน LINE Developers Console
- [ ] Verify SSL certificate
- [ ] Test webhook

---

## 🔧 Post-Deployment

### 1. ตั้งค่า LINE Webhook

ไปที่ [LINE Developers Console](https://developers.line.biz/console/)
1. เลือก Provider & Channel
2. Messaging API → Webhook settings
3. Webhook URL: `https://your-app.railway.app/callback`
4. Verify → Enable webhook

### 2. ทดสอบ API

```bash
# Test generate code
curl -X POST https://your-app.railway.app/api/login/code \
  -H "Content-Type: application/json" \
  -d '{"shop_id":"shop_123"}'

# Test check status
curl https://your-app.railway.app/api/login/status?code=1234
```

### 3. Monitor

**Railway:**
- Logs: Dashboard → Deployments → Logs
- Metrics: Dashboard → Metrics

**Render:**
- Logs: Dashboard → Logs
- Metrics: Dashboard → Metrics

---

## 🐛 Troubleshooting

### Application ไม่ start

```bash
# Check logs
railway logs

# หรือ
fly logs

# ตรวจสอบ:
1. PORT environment variable ตั้งถูกต้องหรือไม่
2. MongoDB URI ถูกต้องหรือไม่
3. Build สำเร็จหรือไม่
```

### MongoDB connection failed

```bash
# ตรวจสอบ:
1. MongoDB Atlas → Network Access → Whitelist 0.0.0.0/0
2. MONGODB_URI ถูกต้อง
3. Database name ถูกต้อง
```

### LINE Webhook ไม่ทำงาน

```bash
# ตรวจสอบ:
1. Webhook URL ถูกต้อง (https://)
2. SSL certificate valid
3. Verify webhook ใน LINE Console
4. ดู logs มี error หรือไม่
```

---

## 💰 Cost Estimation

### Railway (แนะนำ)
- Development: $0 (ใช้ free credit)
- Production: ~$5-10/เดือน

### Render
- Development: $0 (free tier)
- Production: $7/เดือน (Starter)

### Fly.io
- Development: $0 (free tier)
- Production: ~$2-5/เดือน

---

## 📚 Additional Resources

- [Railway Docs](https://docs.railway.app/)
- [Render Docs](https://render.com/docs)
- [Fly.io Docs](https://fly.io/docs/)
- [MongoDB Atlas](https://www.mongodb.com/cloud/atlas)
- [LINE Developers](https://developers.line.biz/)

---

## 🎉 สรุป

**แนะนำ: Railway.app**
- ✅ ง่ายที่สุด
- ✅ Free credit $5
- ✅ Auto-deploy จาก GitHub
- ✅ รองรับ Go HTTP Server เต็มรูปแบบ
- ✅ เหมาะสำหรับ production

**อย่าใช้ Vercel** สำหรับ Go HTTP Server แบบนี้
- ต้องแก้โค้ดเยอะ
- มีข้อจำกัดมาก
- ไม่คุ้มค่า

**Deploy ได้ภายใน 5 นาที! 🚀**
