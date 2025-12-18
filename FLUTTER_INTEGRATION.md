# Flutter Integration Guide - LINE OA Login System

## 📋 สารบัญ
1. [ภาพรวมระบบ](#ภาพรวมระบบ)
2. [API Endpoints](#api-endpoints)
3. [Flutter Project Setup](#flutter-project-setup)
4. [User Flow & UI Design](#user-flow--ui-design)
5. [Implementation Guide](#implementation-guide)
6. [Complete Code Examples](#complete-code-examples)

---

## 🎯 ภาพรวมระบบ

ระบบ Login ผ่าน LINE OA โดยใช้รหัส 4 หลัก เพื่อให้ลูกค้าสามารถ Login เข้าแอพผ่าน LINE OA @bcmember

### กระบวนการทำงาน
```
1. ผู้ใช้กดปุ่ม "เข้าสู่ระบบด้วย LINE OA"
   ↓
2. แอพแสดงรหัส 4 หลัก (BottomSheet/Dialog)
   ↓
3. ผู้ใช้เปิด LINE OA @bcmember และพิมพ์รหัส
   ↓
4. แอพ polling ตรวจสอบสถานะทุก 1 วินาที
   ↓
5. เมื่อสำเร็จ → แอพได้รับ LINE ID และชื่อผู้ใช้
   ↓
6. Navigate ไปหน้าหลักอัตโนมัติ
```

---

## 🔌 API Endpoints

### Base URL
```dart
const String baseUrl = 'http://localhost:8080';
// Production: 'https://your-api-domain.com'
```

### 1. สร้างรหัส Login
```dart
POST /api/login/code

// Request
{
  "shop_id": "shop_123"
}

// Response (Success)
{
  "code": "1234"
}
```

### 2. ตรวจสอบสถานะรหัส
```dart
GET /api/login/status?code=1234

// Response (Pending)
{
  "code": "1234",
  "shop_id": "shop_123",
  "status": "pending",
  "created_at": "2025-12-17T10:00:00Z",
  "expires_at": "2025-12-17T10:05:00Z"
}

// Response (Success)
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

---

## 📦 Flutter Project Setup

### 1. Dependencies (pubspec.yaml)

```yaml
name: your_app
description: LINE OA Login App

environment:
  sdk: '>=3.0.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter

  # HTTP Client
  http: ^1.1.0
  dio: ^5.4.0  # Alternative to http

  # State Management
  provider: ^6.1.1
  # หรือ
  riverpod: ^2.4.9
  # หรือ
  bloc: ^8.1.3

  # UI Components
  flutter_svg: ^2.0.9
  google_fonts: ^6.1.0
  qr_flutter: ^4.1.0

  # Utilities
  intl: ^0.18.1
  shared_preferences: ^2.2.2
  url_launcher: ^6.2.2

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^3.0.1
```

### 2. Install Dependencies
```bash
flutter pub get
```

---

## 🎨 User Flow & UI Design

### Flow Diagram
```
┌─────────────────────────────────────────┐
│  LoginPage (Scaffold)                   │
│                                         │
│  ┌───────────────────────────────────┐ │
│  │  [Logo]                           │ │
│  │                                   │ │
│  │  เข้าสู่ระบบสมาชิก               │ │
│  │                                   │ │
│  │  ┌─────────────────────────────┐ │ │
│  │  │ 📱 เข้าสู่ระบบด้วย LINE OA │ │ │
│  │  └─────────────────────────────┘ │ │
│  │                                   │ │
│  │  หรือ                             │ │
│  │                                   │ │
│  │  [เบอร์โทร TextField]            │ │
│  │  [รหัสผ่าน TextField]            │ │
│  │  [เข้าสู่ระบบ ElevatedButton]    │ │
│  └───────────────────────────────────┘ │
└─────────────────────────────────────────┘
              ↓ (Show BottomSheet)
┌─────────────────────────────────────────┐
│  LoginCodeBottomSheet                   │
│                                         │
│  ┌───────────────────────────────────┐ │
│  │  🔐 ยืนยันตัวตนผ่าน LINE OA     │ │
│  │  ────────────────────────────────│ │
│  │                                   │ │
│  │  กรุณาเปิด LINE OA @bcmember     │ │
│  │  และพิมพ์รหัสด้านล่าง            │ │
│  │                                   │ │
│  │  [QR Code Widget]                 │ │
│  │                                   │ │
│  │  ┌──────────────────────────┐    │ │
│  │  │  1   2   3   4          │    │ │
│  │  └──────────────────────────┘    │ │
│  │                                   │ │
│  │  [📋 คัดลอกรหัส OutlinedButton] │ │
│  │                                   │ │
│  │  ⏱️ หมดอายุใน: 04:45           │ │
│  │  🔄 กำลังรอการยืนยัน...         │ │
│  │                                   │ │
│  │  [ยกเลิก TextButton]             │ │
│  └───────────────────────────────────┘ │
└─────────────────────────────────────────┘
              ↓ (Success)
┌─────────────────────────────────────────┐
│  SuccessDialog                          │
│                                         │
│  ┌───────────────────────────────────┐ │
│  │        ✅                          │ │
│  │  เข้าสู่ระบบสำเร็จ!               │ │
│  │  สวัสดี สมชาย ใจดี                │ │
│  │                                   │ │
│  │  กำลังพาคุณเข้าสู่หน้าหลัก...   │ │
│  └───────────────────────────────────┘ │
└─────────────────────────────────────────┘
              ↓ (Auto navigate)
┌─────────────────────────────────────────┐
│  HomePage                               │
└─────────────────────────────────────────┘
```

---

## 🎨 UI Design Specifications

### Color Scheme
```dart
class AppColors {
  // LINE Brand Color
  static const Color lineGreen = Color(0xFF06C755);
  static const Color lineGreenDark = Color(0xFF05B346);

  // Primary Colors
  static const Color primary = Color(0xFF667EEA);
  static const Color primaryDark = Color(0xFF764BA2);

  // Text Colors
  static const Color textPrimary = Color(0xFF333333);
  static const Color textSecondary = Color(0xFF666666);
  static const Color textHint = Color(0xFF999999);

  // Background
  static const Color background = Color(0xFFF5F5F5);
  static const Color surface = Colors.white;

  // Status Colors
  static const Color success = Color(0xFF4CAF50);
  static const Color error = Color(0xFFF44336);
  static const Color warning = Color(0xFFFF9800);
  static const Color info = Color(0xFF2196F3);
}
```

### Typography
```dart
import 'package:google_fonts/google_fonts.dart';

class AppTextStyles {
  static TextStyle get h1 => GoogleFonts.prompt(
    fontSize: 32,
    fontWeight: FontWeight.bold,
    color: AppColors.textPrimary,
  );

  static TextStyle get h2 => GoogleFonts.prompt(
    fontSize: 24,
    fontWeight: FontWeight.bold,
    color: AppColors.textPrimary,
  );

  static TextStyle get body1 => GoogleFonts.prompt(
    fontSize: 16,
    fontWeight: FontWeight.normal,
    color: AppColors.textPrimary,
  );

  static TextStyle get body2 => GoogleFonts.prompt(
    fontSize: 14,
    fontWeight: FontWeight.normal,
    color: AppColors.textSecondary,
  );

  static TextStyle get button => GoogleFonts.prompt(
    fontSize: 18,
    fontWeight: FontWeight.w600,
    color: Colors.white,
  );

  static TextStyle get codeDigit => GoogleFonts.courierPrime(
    fontSize: 48,
    fontWeight: FontWeight.bold,
    color: AppColors.textPrimary,
  );
}
```

---

## 📁 Project Structure

```
lib/
├── main.dart
├── config/
│   ├── app_colors.dart
│   ├── app_text_styles.dart
│   └── constants.dart
├── models/
│   ├── login_code.dart
│   └── user.dart
├── services/
│   ├── api_service.dart
│   └── auth_service.dart
├── providers/
│   └── auth_provider.dart
├── screens/
│   ├── login_page.dart
│   └── home_page.dart
├── widgets/
│   ├── line_login_button.dart
│   ├── login_code_bottom_sheet.dart
│   ├── code_display.dart
│   ├── countdown_timer.dart
│   └── success_dialog.dart
└── utils/
    ├── validators.dart
    └── helpers.dart
```

---

## 💻 Complete Code Examples

### 1. Models

**models/login_code.dart**
```dart
class LoginCode {
  final String code;
  final String? shopId;
  final String status;
  final String? lineUserId;
  final String? displayName;
  final DateTime createdAt;
  final DateTime expiresAt;

  LoginCode({
    required this.code,
    this.shopId,
    required this.status,
    this.lineUserId,
    this.displayName,
    required this.createdAt,
    required this.expiresAt,
  });

  factory LoginCode.fromJson(Map<String, dynamic> json) {
    return LoginCode(
      code: json['code'] as String,
      shopId: json['shop_id'] as String?,
      status: json['status'] as String,
      lineUserId: json['line_user_id'] as String?,
      displayName: json['display_name'] as String?,
      createdAt: DateTime.parse(json['created_at'] as String),
      expiresAt: DateTime.parse(json['expires_at'] as String),
    );
  }

  bool get isPending => status == 'pending';
  bool get isSuccess => status == 'success';
  bool get isExpired => DateTime.now().isAfter(expiresAt);

  Duration get timeLeft {
    final diff = expiresAt.difference(DateTime.now());
    return diff.isNegative ? Duration.zero : diff;
  }
}
```

**models/user.dart**
```dart
class User {
  final String lineUserId;
  final String displayName;

  User({
    required this.lineUserId,
    required this.displayName,
  });

  factory User.fromJson(Map<String, dynamic> json) {
    return User(
      lineUserId: json['line_user_id'] as String,
      displayName: json['display_name'] as String,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'line_user_id': lineUserId,
      'display_name': displayName,
    };
  }
}
```

---

### 2. Services

**services/api_service.dart**
```dart
import 'dart:convert';
import 'package:http/http.dart' as http;
import '../config/constants.dart';
import '../models/login_code.dart';

class ApiService {
  final String baseUrl;

  ApiService({this.baseUrl = Constants.apiBaseUrl});

  // Generate login code
  Future<String> generateLoginCode(String shopId) async {
    try {
      final response = await http.post(
        Uri.parse('$baseUrl/api/login/code'),
        headers: {'Content-Type': 'application/json'},
        body: json.encode({'shop_id': shopId}),
      );

      if (response.statusCode == 200) {
        final data = json.decode(response.body);
        return data['code'] as String;
      } else {
        throw Exception('Failed to generate code: ${response.statusCode}');
      }
    } catch (e) {
      throw Exception('Error generating code: $e');
    }
  }

  // Check code status
  Future<LoginCode> getCodeStatus(String code) async {
    try {
      final response = await http.get(
        Uri.parse('$baseUrl/api/login/status?code=$code'),
      );

      if (response.statusCode == 200) {
        final data = json.decode(response.body);
        return LoginCode.fromJson(data);
      } else if (response.statusCode == 404) {
        throw Exception('Code not found');
      } else {
        throw Exception('Failed to get status: ${response.statusCode}');
      }
    } catch (e) {
      throw Exception('Error checking status: $e');
    }
  }
}
```

**services/auth_service.dart**
```dart
import 'dart:async';
import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import '../models/user.dart';
import '../models/login_code.dart';
import 'api_service.dart';

class AuthService {
  final ApiService _apiService;
  Timer? _pollingTimer;

  AuthService(this._apiService);

  // Save user to local storage
  Future<void> saveUser(User user) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('user', json.encode(user.toJson()));
  }

  // Get user from local storage
  Future<User?> getUser() async {
    final prefs = await SharedPreferences.getInstance();
    final userJson = prefs.getString('user');

    if (userJson != null) {
      return User.fromJson(json.decode(userJson));
    }
    return null;
  }

  // Logout
  Future<void> logout() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove('user');
  }

  // Check if user is logged in
  Future<bool> isLoggedIn() async {
    final user = await getUser();
    return user != null;
  }

  // Start polling for code status
  Stream<LoginCode> pollCodeStatus(String code) async* {
    while (true) {
      try {
        final loginCode = await _apiService.getCodeStatus(code);
        yield loginCode;

        if (loginCode.isSuccess || loginCode.isExpired) {
          break;
        }

        await Future.delayed(const Duration(seconds: 1));
      } catch (e) {
        yield* Stream.error(e);
        break;
      }
    }
  }

  void dispose() {
    _pollingTimer?.cancel();
  }
}
```

---

### 3. Config

**config/constants.dart**
```dart
class Constants {
  // API Configuration
  static const String apiBaseUrl = 'http://localhost:8080';
  // Production: static const String apiBaseUrl = 'https://your-api.com';

  static const String shopId = 'shop_123';

  // LINE OA
  static const String lineOaId = '@bcmember';
  static const String lineOaUrl = 'https://line.me/R/ti/p/@bcmember';

  // Timing
  static const Duration pollingInterval = Duration(seconds: 1);
  static const Duration codeExpiration = Duration(minutes: 5);
  static const Duration successDelay = Duration(milliseconds: 1500);
}
```

---

### 4. Providers (using Provider package)

**providers/auth_provider.dart**
```dart
import 'package:flutter/foundation.dart';
import '../models/user.dart';
import '../models/login_code.dart';
import '../services/auth_service.dart';
import '../services/api_service.dart';
import '../config/constants.dart';

enum AuthStatus { initial, loading, authenticated, unauthenticated, error }

class AuthProvider with ChangeNotifier {
  final AuthService _authService;
  final ApiService _apiService;

  AuthStatus _status = AuthStatus.initial;
  User? _user;
  String? _errorMessage;
  String? _currentCode;
  LoginCode? _loginCode;
  int _timeLeft = 300; // 5 minutes in seconds

  AuthProvider()
      : _authService = AuthService(ApiService()),
        _apiService = ApiService();

  // Getters
  AuthStatus get status => _status;
  User? get user => _user;
  String? get errorMessage => _errorMessage;
  String? get currentCode => _currentCode;
  LoginCode? get loginCode => _loginCode;
  int get timeLeft => _timeLeft;
  bool get isLoading => _status == AuthStatus.loading;
  bool get isAuthenticated => _status == AuthStatus.authenticated;

  // Check authentication on app start
  Future<void> checkAuth() async {
    try {
      final user = await _authService.getUser();
      if (user != null) {
        _user = user;
        _status = AuthStatus.authenticated;
      } else {
        _status = AuthStatus.unauthenticated;
      }
      notifyListeners();
    } catch (e) {
      _status = AuthStatus.unauthenticated;
      notifyListeners();
    }
  }

  // Generate login code
  Future<void> generateCode() async {
    try {
      _status = AuthStatus.loading;
      _errorMessage = null;
      notifyListeners();

      final code = await _apiService.generateLoginCode(Constants.shopId);
      _currentCode = code;
      _timeLeft = 300;

      // Start polling
      _startPolling();

      _status = AuthStatus.unauthenticated;
      notifyListeners();
    } catch (e) {
      _status = AuthStatus.error;
      _errorMessage = e.toString();
      notifyListeners();
    }
  }

  // Start polling for code status
  void _startPolling() {
    if (_currentCode == null) return;

    _authService.pollCodeStatus(_currentCode!).listen(
      (loginCode) {
        _loginCode = loginCode;
        _timeLeft = loginCode.timeLeft.inSeconds;

        if (loginCode.isSuccess) {
          _onLoginSuccess(loginCode);
        } else if (loginCode.isExpired) {
          _onCodeExpired();
        }

        notifyListeners();
      },
      onError: (error) {
        _status = AuthStatus.error;
        _errorMessage = error.toString();
        notifyListeners();
      },
    );
  }

  // On login success
  void _onLoginSuccess(LoginCode loginCode) {
    if (loginCode.lineUserId != null && loginCode.displayName != null) {
      final user = User(
        lineUserId: loginCode.lineUserId!,
        displayName: loginCode.displayName!,
      );

      _authService.saveUser(user);
      _user = user;
      _status = AuthStatus.authenticated;
      notifyListeners();
    }
  }

  // On code expired
  void _onCodeExpired() {
    _status = AuthStatus.error;
    _errorMessage = 'รหัสหมดอายุ กรุณาลองใหม่อีกครั้ง';
    _currentCode = null;
    notifyListeners();
  }

  // Cancel login
  void cancelLogin() {
    _currentCode = null;
    _loginCode = null;
    _timeLeft = 300;
    _status = AuthStatus.unauthenticated;
    notifyListeners();
  }

  // Logout
  Future<void> logout() async {
    await _authService.logout();
    _user = null;
    _status = AuthStatus.unauthenticated;
    notifyListeners();
  }

  @override
  void dispose() {
    _authService.dispose();
    super.dispose();
  }
}
```

---

### 5. Widgets

**widgets/line_login_button.dart**
```dart
import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';
import '../config/app_colors.dart';
import '../config/app_text_styles.dart';

class LineLoginButton extends StatelessWidget {
  final VoidCallback onPressed;
  final bool isLoading;

  const LineLoginButton({
    Key? key,
    required this.onPressed,
    this.isLoading = false,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      height: 56,
      child: ElevatedButton(
        onPressed: isLoading ? null : onPressed,
        style: ElevatedButton.styleFrom(
          backgroundColor: AppColors.lineGreen,
          foregroundColor: Colors.white,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(12),
          ),
          elevation: 0,
        ),
        child: isLoading
            ? const SizedBox(
                height: 24,
                width: 24,
                child: CircularProgressIndicator(
                  color: Colors.white,
                  strokeWidth: 2,
                ),
              )
            : Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  // LINE Icon (you can use an SVG or Icon)
                  Icon(Icons.chat_bubble, size: 24),
                  const SizedBox(width: 12),
                  Text(
                    'เข้าสู่ระบบด้วย LINE OA',
                    style: AppTextStyles.button,
                  ),
                ],
              ),
      ),
    );
  }
}
```

**widgets/code_display.dart**
```dart
import 'package:flutter/material.dart';
import '../config/app_colors.dart';
import '../config/app_text_styles.dart';

class CodeDisplay extends StatelessWidget {
  final String code;

  const CodeDisplay({
    Key? key,
    required this.code,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final digits = code.split('');

    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: digits.map((digit) => _buildDigit(digit)).toList(),
    );
  }

  Widget _buildDigit(String digit) {
    return Container(
      width: 64,
      height: 80,
      margin: const EdgeInsets.symmetric(horizontal: 4),
      decoration: BoxDecoration(
        color: Colors.grey[100],
        border: Border.all(color: Colors.grey[300]!, width: 2),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Center(
        child: Text(
          digit,
          style: AppTextStyles.codeDigit,
        ),
      ),
    );
  }
}
```

**widgets/countdown_timer.dart**
```dart
import 'package:flutter/material.dart';
import '../config/app_colors.dart';
import '../config/app_text_styles.dart';

class CountdownTimer extends StatelessWidget {
  final int seconds;

  const CountdownTimer({
    Key? key,
    required this.seconds,
  }) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final minutes = seconds ~/ 60;
    final secs = seconds % 60;
    final isWarning = seconds < 60;

    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Icon(
          Icons.timer_outlined,
          size: 20,
          color: isWarning ? AppColors.error : AppColors.textSecondary,
        ),
        const SizedBox(width: 8),
        Text(
          'รหัสหมดอายุใน: ${minutes.toString().padLeft(2, '0')}:${secs.toString().padLeft(2, '0')}',
          style: AppTextStyles.body2.copyWith(
            color: isWarning ? AppColors.error : AppColors.textSecondary,
            fontWeight: isWarning ? FontWeight.w600 : FontWeight.normal,
          ),
        ),
      ],
    );
  }
}
```

**widgets/login_code_bottom_sheet.dart**
```dart
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:url_launcher/url_launcher.dart';
import '../providers/auth_provider.dart';
import '../config/app_colors.dart';
import '../config/app_text_styles.dart';
import '../config/constants.dart';
import 'code_display.dart';
import 'countdown_timer.dart';

class LoginCodeBottomSheet extends StatelessWidget {
  const LoginCodeBottomSheet({Key? key}) : super(key: key);

  static Future<void> show(BuildContext context) {
    return showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (context) => const LoginCodeBottomSheet(),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      padding: EdgeInsets.only(
        bottom: MediaQuery.of(context).viewInsets.bottom,
      ),
      child: Consumer<AuthProvider>(
        builder: (context, authProvider, _) {
          final code = authProvider.currentCode;

          if (code == null) {
            Navigator.pop(context);
            return const SizedBox.shrink();
          }

          return SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                // Handle Bar
                Center(
                  child: Container(
                    width: 40,
                    height: 4,
                    decoration: BoxDecoration(
                      color: Colors.grey[300],
                      borderRadius: BorderRadius.circular(2),
                    ),
                  ),
                ),
                const SizedBox(height: 24),

                // Title
                Row(
                  children: [
                    const Icon(Icons.lock_outline, size: 24),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        'ยืนยันตัวตนผ่าน LINE OA',
                        style: AppTextStyles.h2,
                      ),
                    ),
                    IconButton(
                      onPressed: () {
                        authProvider.cancelLogin();
                        Navigator.pop(context);
                      },
                      icon: const Icon(Icons.close),
                    ),
                  ],
                ),
                const SizedBox(height: 24),

                // Instructions
                _buildInstructions(),
                const SizedBox(height: 24),

                // QR Code
                Center(
                  child: Container(
                    padding: const EdgeInsets.all(16),
                    decoration: BoxDecoration(
                      color: Colors.white,
                      border: Border.all(color: Colors.grey[300]!),
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: QrImageView(
                      data: Constants.lineOaUrl,
                      version: QrVersions.auto,
                      size: 150,
                    ),
                  ),
                ),
                const SizedBox(height: 16),

                // Open LINE Button
                OutlinedButton.icon(
                  onPressed: () => _openLineOA(),
                  icon: const Icon(Icons.chat_bubble_outline),
                  label: const Text('เปิด LINE OA'),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: AppColors.lineGreen,
                    side: const BorderSide(color: AppColors.lineGreen),
                    padding: const EdgeInsets.symmetric(vertical: 12),
                  ),
                ),
                const SizedBox(height: 24),

                // Step 2
                Row(
                  children: [
                    _buildStepNumber(2),
                    const SizedBox(width: 12),
                    Text(
                      'พิมพ์รหัสด้านล่าง',
                      style: AppTextStyles.body1.copyWith(
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 16),

                // Code Display
                CodeDisplay(code: code),
                const SizedBox(height: 16),

                // Copy Button
                OutlinedButton.icon(
                  onPressed: () => _copyCode(context, code),
                  icon: const Icon(Icons.copy, size: 20),
                  label: const Text('คัดลอกรหัส'),
                  style: OutlinedButton.styleFrom(
                    foregroundColor: AppColors.info,
                    side: const BorderSide(color: AppColors.info),
                    padding: const EdgeInsets.symmetric(vertical: 12),
                  ),
                ),
                const SizedBox(height: 16),

                // Timer
                CountdownTimer(seconds: authProvider.timeLeft),
                const SizedBox(height: 16),

                // Loading Indicator
                Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        valueColor: AlwaysStoppedAnimation<Color>(
                          AppColors.lineGreen,
                        ),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Text(
                      'กำลังรอการยืนยัน...',
                      style: AppTextStyles.body2,
                    ),
                  ],
                ),
                const SizedBox(height: 24),

                // Cancel Button
                TextButton(
                  onPressed: () {
                    authProvider.cancelLogin();
                    Navigator.pop(context);
                  },
                  child: Text(
                    'ยกเลิก',
                    style: AppTextStyles.body1.copyWith(
                      color: AppColors.textSecondary,
                    ),
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _buildInstructions() {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: Colors.blue[50],
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          _buildStepNumber(1),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'เปิด LINE OA',
                  style: AppTextStyles.body1.copyWith(
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  'ค้นหา @bcmember หรือสแกน QR Code',
                  style: AppTextStyles.body2,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildStepNumber(int number) {
    return Container(
      width: 32,
      height: 32,
      decoration: BoxDecoration(
        color: AppColors.lineGreen,
        shape: BoxShape.circle,
      ),
      child: Center(
        child: Text(
          '$number',
          style: const TextStyle(
            color: Colors.white,
            fontWeight: FontWeight.w600,
            fontSize: 16,
          ),
        ),
      ),
    );
  }

  Future<void> _copyCode(BuildContext context, String code) async {
    await Clipboard.setData(ClipboardData(text: code));

    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('คัดลอกรหัสสำเร็จ!'),
          duration: Duration(seconds: 2),
          backgroundColor: AppColors.success,
        ),
      );
    }
  }

  Future<void> _openLineOA() async {
    final uri = Uri.parse(Constants.lineOaUrl);
    if (await canLaunchUrl(uri)) {
      await launchUrl(uri, mode: LaunchMode.externalApplication);
    }
  }
}
```

**widgets/success_dialog.dart**
```dart
import 'package:flutter/material.dart';
import '../config/app_colors.dart';
import '../config/app_text_styles.dart';

class SuccessDialog extends StatelessWidget {
  final String displayName;
  final VoidCallback onComplete;

  const SuccessDialog({
    Key? key,
    required this.displayName,
    required this.onComplete,
  }) : super(key: key);

  static Future<void> show(
    BuildContext context, {
    required String displayName,
    required VoidCallback onComplete,
  }) {
    return showDialog(
      context: context,
      barrierDismissible: false,
      builder: (context) => SuccessDialog(
        displayName: displayName,
        onComplete: onComplete,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    // Auto close after 1.5 seconds
    Future.delayed(const Duration(milliseconds: 1500), () {
      if (context.mounted) {
        Navigator.of(context).pop();
        onComplete();
      }
    });

    return Dialog(
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(20),
      ),
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            // Success Icon
            Container(
              width: 80,
              height: 80,
              decoration: BoxDecoration(
                color: AppColors.success.withOpacity(0.1),
                shape: BoxShape.circle,
              ),
              child: const Icon(
                Icons.check_circle,
                size: 60,
                color: AppColors.success,
              ),
            ),
            const SizedBox(height: 24),

            // Success Text
            Text(
              'เข้าสู่ระบบสำเร็จ!',
              style: AppTextStyles.h2.copyWith(
                color: AppColors.success,
              ),
            ),
            const SizedBox(height: 12),

            // Display Name
            Text(
              'สวัสดี $displayName',
              style: AppTextStyles.body1,
            ),
            const SizedBox(height: 8),

            // Redirect Text
            Text(
              'กำลังพาคุณเข้าสู่หน้าหลัก...',
              style: AppTextStyles.body2,
            ),
          ],
        ),
      ),
    );
  }
}
```

---

### 6. Screens

**screens/login_page.dart**
```dart
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../providers/auth_provider.dart';
import '../config/app_colors.dart';
import '../config/app_text_styles.dart';
import '../widgets/line_login_button.dart';
import '../widgets/login_code_bottom_sheet.dart';
import '../widgets/success_dialog.dart';
import 'home_page.dart';

class LoginPage extends StatefulWidget {
  const LoginPage({Key? key}) : super(key: key);

  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> {
  final _phoneController = TextEditingController();
  final _passwordController = TextEditingController();

  @override
  void initState() {
    super.initState();

    // Listen to auth status changes
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final authProvider = context.read<AuthProvider>();
      authProvider.addListener(_handleAuthChange);
    });
  }

  void _handleAuthChange() {
    final authProvider = context.read<AuthProvider>();

    if (authProvider.isAuthenticated && authProvider.user != null) {
      // Show success dialog
      SuccessDialog.show(
        context,
        displayName: authProvider.user!.displayName,
        onComplete: () {
          // Navigate to home page
          Navigator.of(context).pushReplacement(
            MaterialPageRoute(
              builder: (context) => const HomePage(),
            ),
          );
        },
      );
    }
  }

  Future<void> _handleLineLogin() async {
    final authProvider = context.read<AuthProvider>();

    // Generate code
    await authProvider.generateCode();

    if (authProvider.currentCode != null && mounted) {
      // Show bottom sheet with code
      await LoginCodeBottomSheet.show(context);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Container(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [
              AppColors.primary,
              AppColors.primaryDark,
            ],
          ),
        ),
        child: SafeArea(
          child: Center(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(24),
              child: Container(
                constraints: const BoxConstraints(maxWidth: 400),
                padding: const EdgeInsets.all(32),
                decoration: BoxDecoration(
                  color: Colors.white,
                  borderRadius: BorderRadius.circular(20),
                  boxShadow: [
                    BoxShadow(
                      color: Colors.black.withOpacity(0.1),
                      blurRadius: 20,
                      offset: const Offset(0, 10),
                    ),
                  ],
                ),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    // Logo
                    Container(
                      width: 120,
                      height: 120,
                      decoration: BoxDecoration(
                        color: AppColors.primary.withOpacity(0.1),
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.store,
                        size: 60,
                        color: AppColors.primary,
                      ),
                    ),
                    const SizedBox(height: 24),

                    // Title
                    Text(
                      'เข้าสู่ระบบสมาชิก',
                      style: AppTextStyles.h1,
                      textAlign: TextAlign.center,
                    ),
                    const SizedBox(height: 32),

                    // LINE Login Button
                    Consumer<AuthProvider>(
                      builder: (context, authProvider, _) {
                        return LineLoginButton(
                          onPressed: _handleLineLogin,
                          isLoading: authProvider.isLoading,
                        );
                      },
                    ),
                    const SizedBox(height: 24),

                    // Divider
                    Row(
                      children: [
                        const Expanded(child: Divider()),
                        Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 16),
                          child: Text(
                            'หรือ',
                            style: AppTextStyles.body2,
                          ),
                        ),
                        const Expanded(child: Divider()),
                      ],
                    ),
                    const SizedBox(height: 24),

                    // Phone TextField
                    TextField(
                      controller: _phoneController,
                      keyboardType: TextInputType.phone,
                      decoration: InputDecoration(
                        labelText: 'เบอร์โทรศัพท์',
                        prefixIcon: const Icon(Icons.phone),
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                        ),
                      ),
                    ),
                    const SizedBox(height: 16),

                    // Password TextField
                    TextField(
                      controller: _passwordController,
                      obscureText: true,
                      decoration: InputDecoration(
                        labelText: 'รหัสผ่าน',
                        prefixIcon: const Icon(Icons.lock),
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                        ),
                      ),
                    ),
                    const SizedBox(height: 24),

                    // Traditional Login Button
                    SizedBox(
                      height: 56,
                      child: ElevatedButton(
                        onPressed: () {
                          // TODO: Implement traditional login
                        },
                        style: ElevatedButton.styleFrom(
                          backgroundColor: AppColors.primary,
                          shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(12),
                          ),
                        ),
                        child: Text(
                          'เข้าสู่ระบบ',
                          style: AppTextStyles.button,
                        ),
                      ),
                    ),

                    // Error Message
                    Consumer<AuthProvider>(
                      builder: (context, authProvider, _) {
                        if (authProvider.errorMessage != null) {
                          return Padding(
                            padding: const EdgeInsets.only(top: 16),
                            child: Container(
                              padding: const EdgeInsets.all(12),
                              decoration: BoxDecoration(
                                color: AppColors.error.withOpacity(0.1),
                                borderRadius: BorderRadius.circular(8),
                              ),
                              child: Row(
                                children: [
                                  const Icon(
                                    Icons.error_outline,
                                    color: AppColors.error,
                                    size: 20,
                                  ),
                                  const SizedBox(width: 8),
                                  Expanded(
                                    child: Text(
                                      authProvider.errorMessage!,
                                      style: AppTextStyles.body2.copyWith(
                                        color: AppColors.error,
                                      ),
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          );
                        }
                        return const SizedBox.shrink();
                      },
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }

  @override
  void dispose() {
    _phoneController.dispose();
    _passwordController.dispose();
    super.dispose();
  }
}
```

**screens/home_page.dart**
```dart
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import '../providers/auth_provider.dart';
import '../config/app_colors.dart';
import '../config/app_text_styles.dart';
import 'login_page.dart';

class HomePage extends StatelessWidget {
  const HomePage({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    final user = context.watch<AuthProvider>().user;

    return Scaffold(
      appBar: AppBar(
        title: const Text('หน้าหลัก'),
        backgroundColor: AppColors.primary,
        actions: [
          IconButton(
            onPressed: () async {
              final authProvider = context.read<AuthProvider>();
              await authProvider.logout();

              if (context.mounted) {
                Navigator.of(context).pushReplacement(
                  MaterialPageRoute(
                    builder: (context) => const LoginPage(),
                  ),
                );
              }
            },
            icon: const Icon(Icons.logout),
          ),
        ],
      ),
      body: Center(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              // User Avatar
              Container(
                width: 100,
                height: 100,
                decoration: BoxDecoration(
                  color: AppColors.primary.withOpacity(0.1),
                  shape: BoxShape.circle,
                ),
                child: const Icon(
                  Icons.person,
                  size: 60,
                  color: AppColors.primary,
                ),
              ),
              const SizedBox(height: 24),

              // Welcome Text
              Text(
                'ยินดีต้อนรับ',
                style: AppTextStyles.h2,
              ),
              const SizedBox(height: 8),

              // Display Name
              Text(
                user?.displayName ?? 'ผู้ใช้',
                style: AppTextStyles.h1.copyWith(
                  color: AppColors.primary,
                ),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 8),

              // LINE ID
              if (user?.lineUserId != null)
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 8,
                  ),
                  decoration: BoxDecoration(
                    color: Colors.grey[100],
                    borderRadius: BorderRadius.circular(20),
                  ),
                  child: Text(
                    'LINE ID: ${user!.lineUserId}',
                    style: AppTextStyles.body2,
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
```

---

### 7. Main App

**main.dart**
```dart
import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:google_fonts/google_fonts.dart';
import 'providers/auth_provider.dart';
import 'screens/login_page.dart';
import 'screens/home_page.dart';
import 'config/app_colors.dart';

void main() {
  runApp(const MyApp());
}

class MyApp extends StatelessWidget {
  const MyApp({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context) {
    return MultiProvider(
      providers: [
        ChangeNotifierProvider(create: (_) => AuthProvider()..checkAuth()),
      ],
      child: MaterialApp(
        title: 'LINE OA Login',
        debugShowCheckedModeBanner: false,
        theme: ThemeData(
          primarySwatch: Colors.blue,
          scaffoldBackgroundColor: Colors.white,
          textTheme: GoogleFonts.promptTextTheme(),
          colorScheme: ColorScheme.fromSeed(
            seedColor: AppColors.primary,
          ),
          useMaterial3: true,
        ),
        home: Consumer<AuthProvider>(
          builder: (context, authProvider, _) {
            switch (authProvider.status) {
              case AuthStatus.initial:
                return const Scaffold(
                  body: Center(
                    child: CircularProgressIndicator(),
                  ),
                );
              case AuthStatus.authenticated:
                return const HomePage();
              default:
                return const LoginPage();
            }
          },
        ),
      ),
    );
  }
}
```

---

## 🎨 UI Screenshots (Mockup Description)

### Login Page
```
- Gradient background (สีม่วง-ฟ้า)
- Card กลางจอ สีขาว มุมมน
- Logo กลม ตรงกลาง
- ปุ่ม LINE Login สีเขียว มีไอคอน
- Divider "หรือ"
- TextField เบอร์โทร + รหัสผ่าน
- ปุ่มเข้าสู่ระบบแบบปกติ
```

### Login Code Bottom Sheet
```
- Handle bar ด้านบน
- หัวข้อ + ปุ่มปิด
- กล่องคำแนะนำสีฟ้า (Step 1)
- QR Code ตรงกลาง
- ปุ่มเปิด LINE OA
- รหัส 4 หลัก ขนาดใหญ่
- ปุ่มคัดลอกรหัส
- Timer แสดงเวลาหมดอายุ
- Loading spinner + ข้อความรอ
- ปุ่มยกเลิก
```

### Success Dialog
```
- Icon ถูกสีเขียว ขนาดใหญ่
- ข้อความสำเร็จ
- ชื่อผู้ใช้
- ข้อความกำลัง redirect
```

---

## 🚀 Running the App

### Development
```bash
# Run on Android Emulator
flutter run

# Run on iOS Simulator
flutter run -d ios

# Run on specific device
flutter devices
flutter run -d <device_id>
```

### Build for Production
```bash
# Android APK
flutter build apk --release

# Android App Bundle
flutter build appbundle --release

# iOS
flutter build ios --release
```

---

## ✅ Testing Checklist

### Functional Testing
- [ ] Generate code สำเร็จ
- [ ] แสดง BottomSheet พร้อมรหัส
- [ ] QR Code แสดงผลถูกต้อง
- [ ] Copy to clipboard ทำงานได้
- [ ] เปิด LINE OA ได้
- [ ] Countdown timer นับถูกต้อง
- [ ] Polling ทำงานทุก 1 วินาที
- [ ] Login สำเร็จเมื่อพิมพ์รหัสใน LINE
- [ ] Success dialog แสดงผล
- [ ] Navigate ไปหน้าหลักอัตโนมัติ
- [ ] Logout ทำงานถูกต้อง

### UI/UX Testing
- [ ] Responsive บนหน้าจอต่างขนาด
- [ ] Animation สมูท
- [ ] Loading state ชัดเจน
- [ ] Error message แสดงผลถูกต้อง
- [ ] Bottom sheet ปิดได้
- [ ] Keyboard handling

### Platform Testing
- [ ] Android
- [ ] iOS

---

## 🔒 Security Best Practices

1. **HTTPS Only**: ใช้ HTTPS สำหรับ production
2. **Secure Storage**: ใช้ `flutter_secure_storage` สำหรับเก็บ sensitive data
3. **Input Validation**: ตรวจสอบ input ทุกครั้ง
4. **Certificate Pinning**: พิจารณาใช้สำหรับ production

---

## 📱 Platform-Specific Notes

### Android
- อัปเดต `android/app/src/main/AndroidManifest.xml`:
```xml
<uses-permission android:name="android.permission.INTERNET" />
<queries>
    <intent>
        <action android:name="android.intent.action.VIEW" />
        <data android:scheme="https" />
    </intent>
</queries>
```

### iOS
- อัปเดต `ios/Runner/Info.plist`:
```xml
<key>NSAppTransportSecurity</key>
<dict>
    <key>NSAllowsArbitraryLoads</key>
    <true/>
</dict>
<key>LSApplicationQueriesSchemes</key>
<array>
    <string>line</string>
</array>
```

---

## 🎉 สรุป

ระบบนี้ออกแบบมาให้:
- ✅ ใช้งานง่าย มี UX/UI สวยงาม
- ✅ Performance ดี (polling แบบ Stream)
- ✅ State management ด้วย Provider
- ✅ Error handling ครบถ้วน
- ✅ Responsive บนทุกขนาดหน้าจอ
- ✅ รองรับทั้ง Android และ iOS
- ✅ ประหยัด token (6 ข้อความล่าสุด)

**Happy Coding! 🚀**
