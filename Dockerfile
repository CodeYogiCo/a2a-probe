# ── build stage ───────────────────────────────────────────────────────────────
FROM eclipse-temurin:17-jdk-alpine AS builder
WORKDIR /build
COPY gradle gradle
COPY gradlew gradlew
COPY settings.gradle.kts settings.gradle.kts
COPY build.gradle.kts build.gradle.kts
COPY src src
RUN chmod +x gradlew && ./gradlew shadowJar --no-daemon -q

# ── runtime stage ─────────────────────────────────────────────────────────────
FROM eclipse-temurin:17-jre-alpine
WORKDIR /app
COPY --from=builder /build/build/libs/a2a-cli.jar a2a-cli.jar
ENTRYPOINT ["java", "-jar", "a2a-cli.jar"]
