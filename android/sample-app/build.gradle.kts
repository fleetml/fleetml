import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Enrollment key for the sample app: `fleetml.enrollmentKey=...` in android/local.properties
// (gitignored) or `-PfleetmlEnrollmentKey=...` on the Gradle command line.
val localProps = Properties().apply {
    val f = rootProject.file("local.properties")
    if (f.exists()) f.inputStream().use { load(it) }
}
val enrollmentKey: String = (project.findProperty("fleetmlEnrollmentKey") as String?)
    ?: localProps.getProperty("fleetml.enrollmentKey")
    ?: ""

android {
    namespace = "com.fleetml.sample"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.fleetml.sample"
        minSdk = 26
        targetSdk = 34
        versionCode = 1
        versionName = "0.3.0"

        buildConfigField("String", "FLEETML_ENROLLMENT_KEY", "\"$enrollmentKey\"")
    }

    androidResources {
        noCompress += "tflite"
    }

    buildTypes {
        release {
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_1_8
        targetCompatibility = JavaVersion.VERSION_1_8
    }

    kotlinOptions {
        jvmTarget = "1.8"
    }

    buildFeatures {
        buildConfig = true
    }
}

dependencies {
    implementation(project(":fleetml-sdk"))
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("org.tensorflow:tensorflow-lite:2.16.1")
}
