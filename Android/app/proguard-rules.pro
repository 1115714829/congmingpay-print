# Keep Gainscha SDK classes
-keep class com.gainscha.** { *; }
-dontwarn com.gainscha.**

# Keep Gson model classes
-keep class com.congmingpay.android.model.** { *; }
-keep class com.congmingpay.android.api.** { *; }

# Paho MQTT
-keep class org.eclipse.paho.** { *; }
-dontwarn org.eclipse.paho.**

# ZXing
-keep class com.google.zxing.** { *; }
-dontwarn com.google.zxing.**
