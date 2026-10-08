plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
    id("com.google.protobuf")
}

android {
    namespace = "com.fleetml.sdk"
    compileSdk = 34

    defaultConfig {
        minSdk = 26
        targetSdk = 34

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        consumerProguardFiles("consumer-rules.pro")

        buildConfigField("String", "SDK_VERSION", "\"0.1.0\"")
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
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

    testOptions {
        unitTests.isReturnDefaultValues = true
    }

    // The protocol is defined once, in the repository's proto/ directory.
    sourceSets.getByName("main") {
        (this as ExtensionAware).extensions.getByName<SourceDirectorySet>("proto").srcDir("../../proto")
    }
}

protobuf {
    protoc {
        artifact = "com.google.protobuf:protoc:3.25.3"
    }
    plugins {
        create("grpc") {
            artifact = "io.grpc:protoc-gen-grpc-java:1.62.2"
        }
        create("grpckt") {
            artifact = "io.grpc:protoc-gen-grpc-kotlin:1.4.1:jdk8@jar"
        }
    }
    generateProtoTasks {
        all().forEach { task ->
            task.builtins {
                create("java") { option("lite") }
                create("kotlin") { option("lite") }
            }
            task.plugins {
                create("grpc") { option("lite") }
                create("grpckt") { option("lite") }
            }
        }
    }
}

dependencies {
    // gRPC
    implementation("io.grpc:grpc-okhttp:1.62.2")
    implementation("io.grpc:grpc-protobuf-lite:1.62.2")
    implementation("io.grpc:grpc-stub:1.62.2")
    implementation("io.grpc:grpc-kotlin-stub:1.4.1")
    implementation("com.google.protobuf:protobuf-kotlin-lite:3.25.3")

    // Coroutines
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.0")

    // AndroidX
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.security:security-crypto:1.1.0-alpha06")
    // Battery policy: foreground detection and background sync
    implementation("androidx.lifecycle:lifecycle-process:2.6.2")
    implementation("androidx.work:work-runtime-ktx:2.9.0")

    // HTTP
    implementation("com.squareup.okhttp3:okhttp:4.12.0")

    // Model runtimes used by the default validators when the app includes them.
    compileOnly("org.tensorflow:tensorflow-lite:2.16.1")
    compileOnly("com.microsoft.onnxruntime:onnxruntime-android:1.19.2")

    // JSON (for metadata)
    implementation("org.json:json:20240303")

    // ed25519 artifact signatures (the platform API needs API 33)
    implementation("org.bouncycastle:bcprov-jdk18on:1.78.1")

    // Testing
    testImplementation("junit:junit:4.13.2")
    testImplementation("org.robolectric:robolectric:4.12.1")
    testImplementation("io.mockk:mockk:1.13.10")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:1.8.0")
    testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
    testImplementation("androidx.test:core:1.5.0")

    // Instrumented test against a live dev stack (scripts/android-e2e.sh)
    androidTestImplementation("androidx.test.ext:junit:1.1.5")
    androidTestImplementation("androidx.test:runner:1.5.2")
    androidTestImplementation("org.tensorflow:tensorflow-lite:2.16.1")
}
