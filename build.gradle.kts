import com.github.jengelman.gradle.plugins.shadow.tasks.ShadowJar

plugins {
    kotlin("jvm") version "2.0.21"
    kotlin("plugin.serialization") version "2.0.21"
    id("com.github.johnrengelman.shadow") version "8.1.1"
    `maven-publish`
    application
}

group = "com.a2acli"
version = System.getenv("RELEASE_VERSION") ?: "0.1.0-SNAPSHOT"

repositories {
    mavenCentral()
}

val ktorVersion = "2.3.12"
val coroutinesVersion = "1.8.1"
val serializationVersion = "1.7.3"
val cliktVersion = "4.4.0"
val mordantVersion = "2.7.0"
val jlineVersion = "3.26.3"

dependencies {
    // Ktor client
    implementation("io.ktor:ktor-client-core:$ktorVersion")
    implementation("io.ktor:ktor-client-cio:$ktorVersion")
    implementation("io.ktor:ktor-client-websockets:$ktorVersion")
    implementation("io.ktor:ktor-client-content-negotiation:$ktorVersion")
    implementation("io.ktor:ktor-serialization-kotlinx-json:$ktorVersion")
    implementation("io.ktor:ktor-client-logging:$ktorVersion")

    // Coroutines
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:$coroutinesVersion")

    // Serialization
    implementation("org.jetbrains.kotlinx:kotlinx-serialization-json:$serializationVersion")

    // CLI framework
    implementation("com.github.ajalt.clikt:clikt:$cliktVersion")

    // Rich terminal output
    implementation("com.github.ajalt.mordant:mordant:$mordantVersion")
    implementation("com.github.ajalt.mordant:mordant-coroutines:$mordantVersion")

    // Interactive input (readline-style)
    implementation("org.jline:jline-terminal:$jlineVersion")
    implementation("org.jline:jline-reader:$jlineVersion")

    // Logging
    implementation("org.slf4j:slf4j-api:2.0.13")
    implementation("ch.qos.logback:logback-classic:1.5.8")
}

application {
    mainClass.set("com.a2acli.MainKt")
}

tasks.withType<ShadowJar> {
    archiveClassifier.set("")
    archiveFileName.set("a2a-cli.jar")
    mergeServiceFiles()
}

publishing {
    publications {
        create<MavenPublication>("shadow") {
            project.shadow.component(this)
            groupId = "com.a2acli"
            artifactId = "a2a-cli"
            version = project.version.toString()
            pom {
                name.set("a2a-cli")
                description.set("Kotlin CLI client for the A2A Agent-to-Agent Protocol")
                url.set("https://github.com/${System.getenv("GITHUB_REPOSITORY") ?: "codeyogico/a2a-cli-kotlin"}")
                licenses {
                    license {
                        name.set("MIT License")
                        url.set("https://opensource.org/licenses/MIT")
                    }
                }
            }
        }
    }
    repositories {
        maven {
            name = "GitHubPackages"
            url = uri("https://maven.pkg.github.com/${System.getenv("GITHUB_REPOSITORY") ?: "codeyogico/a2a-cli-kotlin"}")
            credentials {
                username = System.getenv("GITHUB_ACTOR") ?: project.findProperty("gpr.user") as String?
                password = System.getenv("GITHUB_TOKEN") ?: project.findProperty("gpr.key") as String?
            }
        }
    }
}

tasks.withType<org.jetbrains.kotlin.gradle.tasks.KotlinCompile> {
    kotlinOptions {
        jvmTarget = "17"
        freeCompilerArgs += "-opt-in=kotlinx.serialization.ExperimentalSerializationApi"
    }
}

tasks.test {
    useJUnitPlatform()
}
