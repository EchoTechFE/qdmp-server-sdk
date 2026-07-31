package io.github.echotechfe.qdmp.groups;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

/** Query parameters for {@code GET /tag/v1/search}. All fields are optional. */
public final class TagSearchParams {

  private final String keyword;
  private final String typeId;
  private final String classification;
  private final List<String> filterOptions;
  private final String offset;
  private final String limit;

  private TagSearchParams(Builder builder) {
    this.keyword = builder.keyword;
    this.typeId = builder.typeId;
    this.classification = builder.classification;
    this.filterOptions = builder.filterOptions;
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
    map.put("typeId", typeId);
    map.put("classification", classification);
    if (filterOptions != null && !filterOptions.isEmpty()) {
      map.put("filterOptions", filterOptions);
    }
    map.put("offset", offset);
    map.put("limit", limit);
    return map;
  }

  /** Builder for {@link TagSearchParams}. */
  public static final class Builder {

    private String keyword;
    private String typeId;
    private String classification;
    private List<String> filterOptions;
    private String offset;
    private String limit;

    private Builder() {}

    public Builder keyword(String keyword) {
      this.keyword = keyword;
      return this;
    }

    public Builder typeId(String typeId) {
      this.typeId = typeId;
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

    public Builder offset(String offset) {
      this.offset = offset;
      return this;
    }

    public Builder limit(String limit) {
      this.limit = limit;
      return this;
    }

    public TagSearchParams build() {
      return new TagSearchParams(this);
    }
  }
}
