package io.github.echotechfe.qdmp.groups;

import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;

/**
 * Query parameters for {@code GET /wishspu/v1/list}. {@code offset}/{@code limit} are required;
 * {@code typeId} is an optional filter (unfiltered when absent).
 */
public final class WishListParams {

  private final String typeId;
  private final String offset;
  private final String limit;

  private WishListParams(Builder builder) {
    this.typeId = builder.typeId;
    this.offset = Objects.requireNonNull(builder.offset, "offset must not be null");
    this.limit = Objects.requireNonNull(builder.limit, "limit must not be null");
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
    map.put("offset", offset);
    map.put("limit", limit);
    return map;
  }

  /** Builder for {@link WishListParams}. */
  public static final class Builder {

    private String typeId;
    private String offset;
    private String limit;

    private Builder() {}

    public Builder typeId(String typeId) {
      this.typeId = typeId;
      return this;
    }

    public Builder offset(String offset) {
      this.offset = offset;
      return this;
    }

    public Builder limit(String limit) {
      this.limit = limit;
      return this;
    }

    public WishListParams build() {
      return new WishListParams(this);
    }
  }
}
