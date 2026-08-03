package io.github.echotechfe.qdmp;

import io.github.echotechfe.qdmp.auth.UserCredentialOptions;
import io.github.echotechfe.qdmp.auth.UserCredentialSession;
import io.github.echotechfe.qdmp.auth.UserCredentialSnapshot;
import io.github.echotechfe.qdmp.generated.GenaiDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.GenaiGenerate200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.GenaiGenerateRequest;
import io.github.echotechfe.qdmp.generated.IslandDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkAdd200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkAddRequest;
import io.github.echotechfe.qdmp.generated.MarkDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkList200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.MarkSearch200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.SpuDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.SpuSearch200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.TagDetail200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.TagSearch200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.UserMe200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.WishAdd200ResponseAllOfData;
import io.github.echotechfe.qdmp.generated.WishAddRequest;
import io.github.echotechfe.qdmp.generated.WishCancelRequest;
import io.github.echotechfe.qdmp.generated.WishList200ResponseAllOfData;
import io.github.echotechfe.qdmp.groups.GenaiGroup;
import io.github.echotechfe.qdmp.groups.IslandGroup;
import io.github.echotechfe.qdmp.groups.MarkGroup;
import io.github.echotechfe.qdmp.groups.MarkSearchParams;
import io.github.echotechfe.qdmp.groups.SpuGroup;
import io.github.echotechfe.qdmp.groups.SpuSearchParams;
import io.github.echotechfe.qdmp.groups.TagGroup;
import io.github.echotechfe.qdmp.groups.TagSearchParams;
import io.github.echotechfe.qdmp.groups.UserGroup;
import io.github.echotechfe.qdmp.groups.WishListParams;
import io.github.echotechfe.qdmp.groups.WishSpuGroup;

/**
 * A client bound to one user-authorization credential, obtained from {@link
 * QdmpClient#withUserCredential(UserCredentialOptions)}.
 *
 * <p>Its business methods take no {@link QdmpContext}: the credential is held by the client, which
 * renews it 300 seconds ahead of a known expiry and again -- exactly once, then retrying the call
 * -- whenever a call comes back as an HTTP 401 with business code {@code 10005}/{@code 10006}. The
 * root {@link QdmpClient}'s own groups are unaffected and still take a per-call context.
 *
 * <p>Every accessor returns a thin binding over the corresponding group on the root client, so
 * there is exactly one implementation of each operation and exactly one implementation of the
 * renewal policy (in {@link QdmpTransport}) behind both entry points.
 */
public final class QdmpUserClient {

  private final UserCredentialSession session;
  private final BoundUserGroup user;
  private final BoundIslandGroup island;
  private final BoundSpuGroup spu;
  private final BoundTagGroup tag;
  private final BoundMarkGroup mark;
  private final BoundWishSpuGroup wishspu;
  private final BoundGenaiGroup genai;

  QdmpUserClient(QdmpClient client, UserCredentialSession session) {
    this.session = session;
    QdmpContext ctx = QdmpContext.boundTo(session);
    this.user = new BoundUserGroup(client.user(), ctx);
    this.island = new BoundIslandGroup(client.island(), ctx);
    this.spu = new BoundSpuGroup(client.spu(), ctx);
    this.tag = new BoundTagGroup(client.tag(), ctx);
    this.mark = new BoundMarkGroup(client.mark(), ctx);
    this.wishspu = new BoundWishSpuGroup(client.wishspu(), ctx);
    this.genai = new BoundGenaiGroup(client.genai(), ctx);
  }

  /**
   * Returns the credential as it stands right now, for a caller that wants to persist it (e.g.
   * before the process exits) after the SDK has renewed it.
   *
   * @return an immutable snapshot; a later renewal replaces it rather than mutating it, so a
   *     snapshot taken earlier keeps reporting the token that was current when it was taken
   */
  public UserCredentialSnapshot credential() {
    return session.snapshot();
  }

  /**
   * Returns the {@code user} group, bound to this client's credential.
   *
   * @return the bound {@code user} group
   */
  public BoundUserGroup user() {
    return user;
  }

  /**
   * Returns the {@code island} group, bound to this client's credential.
   *
   * @return the bound {@code island} group
   */
  public BoundIslandGroup island() {
    return island;
  }

  /**
   * Returns the {@code spu} group, bound to this client's credential.
   *
   * @return the bound {@code spu} group
   */
  public BoundSpuGroup spu() {
    return spu;
  }

  /**
   * Returns the {@code tag} group, bound to this client's credential.
   *
   * @return the bound {@code tag} group
   */
  public BoundTagGroup tag() {
    return tag;
  }

  /**
   * Returns the {@code mark} group, bound to this client's credential.
   *
   * @return the bound {@code mark} group
   */
  public BoundMarkGroup mark() {
    return mark;
  }

  /**
   * Returns the {@code wishspu} group, bound to this client's credential.
   *
   * @return the bound {@code wishspu} group
   */
  public BoundWishSpuGroup wishspu() {
    return wishspu;
  }

  /**
   * Returns the {@code genai} group, bound to this client's credential.
   *
   * @return the bound {@code genai} group
   */
  public BoundGenaiGroup genai() {
    return genai;
  }

  /** The {@code user} group with this client's credential supplied for you. */
  public static final class BoundUserGroup {

    private final UserGroup delegate;
    private final QdmpContext ctx;

    private BoundUserGroup(UserGroup delegate, QdmpContext ctx) {
      this.delegate = delegate;
      this.ctx = ctx;
    }

    /**
     * Fetches the authorized user's profile.
     *
     * @return the user's profile
     */
    public UserMe200ResponseAllOfData me() {
      return delegate.me(ctx);
    }
  }

  /** The {@code island} group with this client's credential supplied for you. */
  public static final class BoundIslandGroup {

    private final IslandGroup delegate;
    private final QdmpContext ctx;

    private BoundIslandGroup(IslandGroup delegate, QdmpContext ctx) {
      this.delegate = delegate;
      this.ctx = ctx;
    }

    /**
     * Fetches an island's details.
     *
     * @param id the island ID
     * @return the island details
     */
    public IslandDetail200ResponseAllOfData detail(String id) {
      return delegate.detail(ctx, id);
    }
  }

  /** The {@code spu} group with this client's credential supplied for you. */
  public static final class BoundSpuGroup {

    private final SpuGroup delegate;
    private final QdmpContext ctx;

    private BoundSpuGroup(SpuGroup delegate, QdmpContext ctx) {
      this.delegate = delegate;
      this.ctx = ctx;
    }

    /**
     * Fetches a SPU's details.
     *
     * @param id the SPU ID
     * @return the SPU details
     */
    public SpuDetail200ResponseAllOfData detail(String id) {
      return delegate.detail(ctx, id);
    }

    /**
     * Searches SPUs.
     *
     * @param params the search filters
     * @return the SPU list and total count
     */
    public SpuSearch200ResponseAllOfData search(SpuSearchParams params) {
      return delegate.search(ctx, params);
    }
  }

  /** The {@code tag} group with this client's credential supplied for you. */
  public static final class BoundTagGroup {

    private final TagGroup delegate;
    private final QdmpContext ctx;

    private BoundTagGroup(TagGroup delegate, QdmpContext ctx) {
      this.delegate = delegate;
      this.ctx = ctx;
    }

    /**
     * Fetches a tag's details.
     *
     * @param id the tag ID
     * @return the tag details
     */
    public TagDetail200ResponseAllOfData detail(String id) {
      return delegate.detail(ctx, id);
    }

    /**
     * Searches tags.
     *
     * @param params the search filters
     * @return the tag list and total count
     */
    public TagSearch200ResponseAllOfData search(TagSearchParams params) {
      return delegate.search(ctx, params);
    }
  }

  /** The {@code mark} group with this client's credential supplied for you. */
  public static final class BoundMarkGroup {

    private final MarkGroup delegate;
    private final QdmpContext ctx;

    private BoundMarkGroup(MarkGroup delegate, QdmpContext ctx) {
      this.delegate = delegate;
      this.ctx = ctx;
    }

    /**
     * Adds a mark (optionally with a rating) for a SPU.
     *
     * @param request the SPU to mark and optional rating
     * @return the created mark's ID
     */
    public MarkAdd200ResponseAllOfData add(MarkAddRequest request) {
      return delegate.add(ctx, request);
    }

    /**
     * Lists the user's marks, paginated.
     *
     * @param limit page size (max 100)
     * @param offset page offset (max 100)
     * @return the mark list and total count
     */
    public MarkList200ResponseAllOfData list(String limit, String offset) {
      return delegate.list(ctx, limit, offset);
    }

    /**
     * Searches the user's marks by category, paginated.
     *
     * @param params the search filters
     * @return the mark list and total count
     */
    public MarkSearch200ResponseAllOfData search(MarkSearchParams params) {
      return delegate.search(ctx, params);
    }

    /**
     * Fetches a single mark's details, including its paginated history.
     *
     * @param id the mark spu info ID
     * @param limit history page size (max 100)
     * @param offset history page offset (max 100)
     * @return the mark details
     */
    public MarkDetail200ResponseAllOfData detail(String id, String limit, String offset) {
      return delegate.detail(ctx, id, limit, offset);
    }
  }

  /** The {@code wishspu} group with this client's credential supplied for you. */
  public static final class BoundWishSpuGroup {

    private final WishSpuGroup delegate;
    private final QdmpContext ctx;

    private BoundWishSpuGroup(WishSpuGroup delegate, QdmpContext ctx) {
      this.delegate = delegate;
      this.ctx = ctx;
    }

    /**
     * Batch-adds SPUs to the user's wish list.
     *
     * @param request the target IDs and type
     * @return the number of successful additions
     */
    public WishAdd200ResponseAllOfData add(WishAddRequest request) {
      return delegate.add(ctx, request);
    }

    /**
     * Batch-removes SPUs from the user's wish list.
     *
     * @param request the target IDs and type
     * @return the number of successful removals
     */
    public WishAdd200ResponseAllOfData cancel(WishCancelRequest request) {
      return delegate.cancel(ctx, request);
    }

    /**
     * Lists the user's wish list, paginated.
     *
     * @param params the list filters
     * @return the wish list and total count
     */
    public WishList200ResponseAllOfData list(WishListParams params) {
      return delegate.list(ctx, params);
    }
  }

  /** The {@code genai} group with this client's credential supplied for you. */
  public static final class BoundGenaiGroup {

    private final GenaiGroup delegate;
    private final QdmpContext ctx;

    private BoundGenaiGroup(GenaiGroup delegate, QdmpContext ctx) {
      this.delegate = delegate;
      this.ctx = ctx;
    }

    /**
     * Creates an asynchronous GenAI generation task.
     *
     * @param request the model, conversation contents, and optional generation settings
     * @return the created task's ID
     */
    public GenaiGenerate200ResponseAllOfData generate(GenaiGenerateRequest request) {
      return delegate.generate(ctx, request);
    }

    /**
     * Queries the status/result of an asynchronous GenAI generation task.
     *
     * @param id the task ID returned by {@link #generate}
     * @return the task's current status and, once completed, its result
     */
    public GenaiDetail200ResponseAllOfData detail(String id) {
      return delegate.detail(ctx, id);
    }
  }
}
