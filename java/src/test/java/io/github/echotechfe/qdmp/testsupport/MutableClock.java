package io.github.echotechfe.qdmp.testsupport;

import java.time.Clock;
import java.time.Instant;
import java.time.ZoneId;

/**
 * A {@link Clock} whose {@link #instant()} can be advanced on demand, so tests can
 * deterministically cross the SDK's token-refresh buffer without sleeping in real time.
 */
public final class MutableClock extends Clock {

  private volatile Instant instant;
  private final ZoneId zone;

  private MutableClock(Instant instant, ZoneId zone) {
    this.instant = instant;
    this.zone = zone;
  }

  public static MutableClock startingAt(Instant instant) {
    return new MutableClock(instant, ZoneId.of("UTC"));
  }

  public void advanceSeconds(long seconds) {
    this.instant = this.instant.plusSeconds(seconds);
  }

  @Override
  public ZoneId getZone() {
    return zone;
  }

  @Override
  public Clock withZone(ZoneId zone) {
    return new MutableClock(instant, zone);
  }

  @Override
  public Instant instant() {
    return instant;
  }
}
