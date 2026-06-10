FROM eclipse-temurin:17-jre-alpine
WORKDIR /app
COPY build/libs/a2a-cli.jar a2a-cli.jar
ENTRYPOINT ["java", "-jar", "a2a-cli.jar"]
