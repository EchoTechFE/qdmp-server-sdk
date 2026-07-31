package io.github.echotechfe.qdmp.groups;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/**
 * Query parameters for {@code GET /spu/v1/search}. All fields are optional; {@code uint64}-typed
 * filters (e.g. {@code typeId}) are wire-encoded as strings, per the project's int64/uint64
 * convention.
 */
public final class SpuSearchParams {

  private final String keyword;
  private final String ipTag;
  private final String typeId;
  private final String megaId;
  private final String classification;
  private final List<String> filterOptions;
  private final Boolean onlyMega;
  private final Boolean onlySpu;
  private final String offset;
  private final String limit;

  private SpuSearchParams(Builder builder) {
    this.keyword = builder.keyword;
    this.ipTag = builder.ipTag;
    this.typeId = builder.typeId;
    this.megaId = builder.megaId;
    this.classification = builder.classification;
    this.filterOptions = builder.filterOptions;
    this.onlyMega = builder.onlyMega;
    this.onlySpu = builder.onlySpu;
    this.offset = builder.offset;
    this.limit = builder.limit;
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
    map.put("keyword", keyword);
    map.put("ipTag", ipTag);
    map.put("typeId", typeId);
    map.put("megaId", megaId);
    map.put("classification", classification);
    if (filterOptions != null && !filterOptions.isEmpty()) {
      map.put("filterOptions", filterOptions);
    }
    map.put("onlyMega", onlyMega);
    map.put("onlySpu", onlySpu);
    map.put("offset", offset);
    map.put("limit", limit);
    return map;
  }

  /** Builder for {@link SpuSearchParams}. */
  public static final class Builder {

    private String keyword;
    private String ipTag;
    private String typeId;
    private String megaId;
    private String classification;
    private List<String> filterOptions;
    private Boolean onlyMega;
    private Boolean onlySpu;
    private String offset;
    private String limit;

    private Builder() {}

    public Builder keyword(String keyword) {
      this.keyword = keyword;
      return this;
    }

    public Builder ipTag(String ipTag) {
      this.ipTag = ipTag;
      return this;
    }

    public Builder typeId(String typeId) {
      this.typeId = typeId;
      return this;
    }

    public Builder megaId(String megaId) {
      this.megaId = megaId;
      return this;
    }

    public Builder classification(String classification) {
      this.classification = classification;
      return this;
    }

    public Builder filterOptions(List<String> filterOptions) {
      this.filterOptions = filterOptions;
      return this;
    }

    public Builder onlyMega(Boolean onlyMega) {
      this.onlyMega = onlyMega;
      return this;
    }

    public Builder onlySpu(Boolean onlySpu) {
      this.onlySpu = onlySpu;
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

    public SpuSearchParams build() {
      return new SpuSearchParams(this);
    }
  }
}
