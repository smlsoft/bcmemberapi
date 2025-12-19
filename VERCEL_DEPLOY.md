# Vercel Deployment Guide

## ⚠️ สำคัญ: ข้อจำกัดของ Vercel

**Vercel Serverless มีข้อจำกัด:**
- ⏱️ Function timeout: **10 วินาที** (free), 60 วินาที (Pro)
- ❄️ Cold start ช้า (~1-3 วินาที)
- 🔄 ไม่มี persistent connections (MongoDB เปิด-ปิดทุกครั้ง)
- 💰 ราคาแพงถ้าใช้บ่อย

**แนะนำใช้ Railway/Render แทน** ถ้าต้องการ production

---

## 🚀 Deploy to Vercel

### 1. Install Vercel CLI

```bash
npm install -g vercel
```

### 2. Login to Vercel

```bash
vercel login
```

### 3. Push to Git (if not already)

```bash
git add .
git commit -m "Prepare for Vercel deployment"
git push origin main
```

### 4. Deploy

```bash
# Deploy from local
cd d:/BC/bcmemberapi
vercel

# หรือ deploy from GitHub
# ไปที่ vercel.com → Import Project → Select bcmemberapi repo
```

### 5. ตั้งค่า Environment Variables

ใน Vercel Dashboard → Settings → Environment Variables:

```bash
LINE_CHANNEL_SECRET=1aa5bc403aef2a07332c1ce68ed4182b
LINE_CHANNEL_TOKEN=iRJORAkhPonQv76BrXYoWlzq6TS3ntV/Tw7XtZyhomWgyLHF7EKtVlOYhRN62VT+QJXNM31pLUxH0p55Ca2HcXhBKTlpsSNWY1iEuTm89z79voHkhTrMUeh2p6x8BTyaxpczoclNt58ZOLsG3caJagdB04t89/1O/w1cDnyilFU=
GEMINI_API_KEY=AIzaSyDgliR378fVGSV8Cf7jcJAWoRktjSsm66k
MONGODB_URI=mongodb+srv://jaturapornchai:Jead%401911@jaturapornchai.bbzktpw.mongodb.net/bcai_documents?retryWrites=true&w=majority
MONGODB_DBNAME=bcmember
```

### 6. Redeploy

```bash
vercel --prod
```

---

## 📋 API Endpoints (After Deployment)

Base URL: `https://your-project.vercel.app`

### 1. Generate Login Code
```bash
curl -X POST https://your-project.vercel.app/api/login/code \
  -H "Content-Type: application/json" \
  -d '{"shop_id":"shop_123"}'
```

### 2. Check Login Status
```bash
curl https://your-project.vercel.app/api/login/status?code=1234
```

### 3. LINE Webhook
```
POST https://your-project.vercel.app/api/callback
```

---

## ⚙️ ตั้งค่า LINE Webhook

1. ไปที่ [LINE Developers Console](https://developers.line.biz/console/)
2. เลือก Provider & Channel
3. Messaging API → Webhook settings
4. Webhook URL: `https://your-project.vercel.app/api/callback`
5. Verify → Enable webhook
6. Auto-reply messages: **ปิด**
7. Greeting messages: **เปิด** (ถ้าต้องการ)

---

## 🐛 Troubleshooting

### Function Timeout Error

```json
{
  "error": "FUNCTION_INVOCATION_TIMEOUT"
}
```

**สาเหตุ:**
- MongoDB connection ช้า
- AI response ช้า
- Cold start

**แก้ไข:**
1. Upgrade to Pro plan (60s timeout)
2. หรือใช้ Railway/Render แทน

### MongoDB Connection Failed

```json
{
  "error": "Database connection failed"
}
```

**แก้ไข:**
1. ตรวจสอบ `MONGODB_URI` ใน Environment Variables
2. MongoDB Atlas → Network Access → Whitelist `0.0.0.0/0`
3. ตรวจสอบ username/password ถูกต้อง

### CORS Error

ถ้า Frontend เจอ CORS error:

**แก้ไข:**
- Headers ตั้งค่าไว้แล้วใน code
- ตรวจสอบว่า Frontend เรียก URL ถูกต้อง
- ใช้ `https://` ไม่ใช่ `http://`

---

## 📊 Vercel Limits (Free Tier)

| Limit | Free | Pro |
|-------|------|-----|
| Function Timeout | 10s | 60s |
| Bandwidth | 100GB | 1TB |
| Serverless Function Executions | 100GB-Hrs | Unlimited |
| Build Time | 6,000 minutes | Unlimited |

---

## 💰 Cost Estimation

### Free Tier
- ✅ เหมาะสำหรับ development/testing
- ✅ 100GB bandwidth/เดือน
- ⚠️ Function timeout 10 วินาที (อาจไม่พอ)

### Pro Plan ($20/เดือน)
- ✅ 60 วินาที timeout
- ✅ Better performance
- ✅ Priority support

---

## 🔍 Monitoring & Logs

### View Logs

```bash
vercel logs
```

หรือใน Vercel Dashboard:
- Deployments → Select deployment → View Logs

### Monitor Performance

Vercel Dashboard:
- Analytics → Performance
- Logs → Real-time

---

## 🎯 Best Practices

1. **MongoDB Connection Pooling**
   - ใช้ connection string ที่มี `maxPoolSize=10`
   - Close connections หลังใช้งาน

2. **Error Handling**
   - Catch all errors
   - Return proper HTTP status codes
   - Log errors สำหรับ debugging

3. **Timeouts**
   - Set context timeout 8 วินาที (เหลือ buffer 2 วินาที)
   - Handle timeout gracefully

4. **CORS**
   - Set CORS headers ถูกต้อง
   - Handle OPTIONS requests

---

## ✅ Deployment Checklist

- [ ] Push code to GitHub
- [ ] Deploy to Vercel
- [ ] ตั้งค่า Environment Variables
- [ ] ทดสอบ API endpoints
- [ ] ตั้งค่า LINE Webhook URL
- [ ] Verify LINE Webhook
- [ ] ทดสอบ Login flow
- [ ] ทดสอบ AI Chatbot
- [ ] Monitor logs

---

## 🔄 CI/CD (Auto Deploy)

Vercel auto-deploy ทุกครั้งที่ push to GitHub:

```bash
# Push to main branch
git add .
git commit -m "Update"
git push origin main

# Vercel จะ auto-deploy ภายใน 1-2 นาที
```

View deployment status:
- Vercel Dashboard → Deployments
- ได้รับ notification ทาง email

---

## 🌐 Custom Domain (Optional)

### Add Custom Domain

1. Vercel Dashboard → Settings → Domains
2. Add domain: `api.yourdomain.com`
3. ตั้งค่า DNS:
   - Type: `CNAME`
   - Name: `api`
   - Value: `cname.vercel-dns.com`
4. Wait for DNS propagation (~5-10 นาที)

### Update LINE Webhook

Update webhook URL เป็น:
```
https://api.yourdomain.com/api/callback
```

---

## 📝 Files Created for Vercel

```
bcmemberapi/
├── api/
│   ├── login/
│   │   ├── code.go      ✅ POST /api/login/code
│   │   └── status.go    ✅ GET /api/login/status
│   └── callback.go      ✅ POST /api/callback
├── vercel.json          ✅ Vercel configuration
└── VERCEL_DEPLOY.md     ✅ This file
```

---

## 🚀 Quick Commands

```bash
# Deploy to development
vercel

# Deploy to production
vercel --prod

# View logs
vercel logs

# View domains
vercel domains ls

# Add environment variable
vercel env add

# Remove deployment
vercel rm [deployment-url]
```

---

## 📞 Support

**Vercel Issues:**
- [Vercel Documentation](https://vercel.com/docs)
- [Vercel Community](https://github.com/vercel/vercel/discussions)

**Project Issues:**
- Email: jaturapornchai@gmail.com
- LINE OA: @bcmember

---

## ⚡ สรุป

**Vercel Serverless Functions:**
- ✅ Deploy ง่าย
- ✅ Auto-scaling
- ✅ Free tier มี
- ⚠️ Timeout จำกัด (10s free, 60s pro)
- ⚠️ Cold start ช้า
- ⚠️ MongoDB connection overhead

**แนะนำ Railway/Render** ถ้าต้องการ:
- ✅ ไม่มี timeout limit
- ✅ Persistent connections
- ✅ Better performance
- ✅ ราคาถูกกว่า

**แต่ Vercel ก็ใช้ได้ดีสำหรับ:**
- Development/Testing
- Low traffic applications
- ถ้า upgrade Pro plan

---

**Deploy เสร็จแล้ว! 🎉**
