package io.github.echotechfe.qdmp.groups;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/**
 * Query parameters for {@code GET /mark/v1/me/search}. {@code limit}/{@code offset} are required;
 * {@code typeId} is an optional filter.
 */
public final class MarkSearchParams {

  private final String typeId;
  private final String limit;
  private final String offset;

  private MarkSearchParams(Builder builder) {
    this.typeId = builder.typeId;
    this.limit = Objects.requireNonNull(builder.limit, "limit must not be null");
    this.offset = Objects.requireNonNull(builder.offset, "offset must not be null");
  }

  /**
   * Creates a new builder.
   *
   * @return a new {@link Builder}
   */
  public static Builder builder() {
    return new Builder();
  }

  Map<String, Object> toQueryMap() {
    Map<String, Object> map = new LinkedHashMap<>();
    map.put("typeId", typeId);
    map.put("limit", limit);
    map.put("offset", offset);
    return map;
  }

  /** Builder for {@link MarkSearchParams}. */
  public static final class Builder {

    private String typeId;
    private String limit;
    private String offset;

    private Builder() {}

    public Builder typeId(String typeId) {
      this.typeId = typeId;
      return this;
    }

    public Builder limit(String limit) {
      this.limit = limit;
      return this;
    }

    public Builder offset(String offset) {
      this.offset = offset;
      return this;
    }

    public MarkSearchParams build() {
      return new MarkSearchParams(this);
    }
  }
}
