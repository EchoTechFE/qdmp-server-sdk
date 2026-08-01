import com.diffplug.gradle.spotless.SpotlessExtension

plugins {
  `java-library`
  checkstyle
  jacoco
  id("com.diffplug.spotless") version "6.25.0"
  // Publishes to the new Sonatype Central Portal (central.sonatype.com) —
  // plain `maven-publish` can't talk to its upload API. Not wired up yet:
  // needs ORG_GRADLE_PROJECT_mavenCentralUsername/Password (Central Portal
  // user token, not your account password) and a GPG signing key, none of
  // which exist yet (see README's "发布现状"). `./gradlew publish` will just
  // fail with a clear auth error until those are configured — that's
  // intentional for this skeleton stage.
  id("com.vanniktech.maven.publish") version "0.33.0"
}

group = "io.github.echotechfe"
version = "0.1.1"

java {
  sourceCompatibility = JavaVersion.VERSION_11
  targetCompatibility = JavaVersion.VERSION_11
  withSourcesJar()
}

// openapi-generator output (models only). Regenerate with:
// java -jar /tmp/tools/openapi-generator-cli.jar generate \
//   -i ../shared/openapi.yaml -g java -o generated \
//   --global-property models,modelTests=false,modelDocs=false \
//   --model-package=io.github.echotechfe.qdmp.generated \
//   --library=resttemplate \
//   --additional-properties=hideGenerationTimestamp=true,dateLibrary=java8,openApiNullable=false
// See java/generated/README.md for why --library=resttemplate and openApiNullable=false were chosen.
sourceSets {
  main {
    java.srcDir("generated/src/main/java")
  }
}

repositories {
  mavenCentral()
}

val jacksonVersion = "2.17.2"

dependencies {
  api("com.fasterxml.jackson.core:jackson-databind:$jacksonVersion")
  implementation("com.squareup.okhttp3:okhttp:4.12.0")
  // Required at compile time by the openapi-generator "resttemplate" model templates
  // (javax.annotation.Nonnull/Nullable/Generated), even in models-only mode.
  implementation("com.google.code.findbugs:jsr305:3.0.2")
  implementation("javax.annotation:javax.annotation-api:1.3.2")

  testImplementation(platform("org.junit:junit-bom:5.11.0"))
  testImplementation("org.junit.jupiter:junit-jupiter")
  testRuntimeOnly("org.junit.platform:junit-platform-launcher")
  testImplementation("com.squareup.okhttp3:mockwebserver:4.12.0")
  testImplementation("org.assertj:assertj-core:3.26.3")
}

tasks.test {
  useJUnitPlatform()
  finalizedBy(tasks.jacocoTestReport)
}

tasks.jacocoTestReport {
  dependsOn(tasks.test)
  reports {
    xml.required.set(true)
    html.required.set(true)
  }
  // Generated DTOs have no hand-written logic to cover; excluding them keeps the
  // report focused on src/main/java's actual coverage instead of diluting it with
  // openapi-generator output.
  classDirectories.setFrom(
    files(
      classDirectories.files.map {
        fileTree(it) { exclude("**/generated/**") }
      },
    ),
  )
}

tasks.register<Exec>("codegen") {
  description =
    "Regenerates generated/src/main/java/.../generated/{RouteMeta,KnownErrorCodes}.java from " +
      "shared/generated/{route-meta,error-codes}.json (see scripts/gen-route-meta.mjs)."
  group = "build"
  commandLine("node", "scripts/gen-route-meta.mjs")
}

checkstyle {
  toolVersion = "10.17.0"
  // google_checks.xml ships inside the checkstyle tool jar itself; pull it out instead of
  // vendoring a copy that could drift from the toolVersion above.
  config = resources.text.fromArchiveEntry(
    configurations.checkstyle.get().files.first { it.name.startsWith("checkstyle-") },
    "google_checks.xml",
  )
  maxWarnings = 0
}

tasks.withType<Checkstyle>().configureEach {
  // Generated DTOs are not hand-maintained; don't lint them.
  exclude("**/generated/**")
}

configure<SpotlessExtension> {
  java {
    target("src/main/java/**/*.java", "src/test/java/**/*.java")
    googleJavaFormat()
  }
}

mavenPublishing {
  publishToMavenCentral()
  signAllPublications()

  coordinates("io.github.echotechfe", "qdmp-server-sdk", version.toString())

  pom {
    name.set("qdmp-server-sdk")
    description.set("千岛（qdmp）OpenAPI 官方 Java Server SDK")
    url.set("https://github.com/EchoTechFE/qdmp-server-sdk")
    licenses {
      license {
        name.set("MIT")
        url.set("https://github.com/EchoTechFE/qdmp-server-sdk/blob/main/LICENSE")
      }
    }
    developers {
      developer {
        name.set("EchoTechFE")
        url.set("https://github.com/EchoTechFE")
      }
    }
    scm {
      url.set("https://github.com/EchoTechFE/qdmp-server-sdk")
      connection.set("scm:git:https://github.com/EchoTechFE/qdmp-server-sdk.git")
    }
  }
}
