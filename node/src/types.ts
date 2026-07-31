/**
 * Hand-authored request/response shapes for the 17 qdmp OpenAPI operations,
 * mirroring shared/openapi.yaml (int64/uint64 fields are string on the
 * wire — see that file's field-level notes for the real-world evidence).
 * These are intentionally plain, narrow interfaces rather than derived from
 * src/generated/schema.d.ts: this SDK only generates types/DTOs from the
 * spec, it does not generate a full client, and the hand-written
 * groups/*.ts call sites are the actual
 * source of truth for what shape each method returns to callers.
 */

/** Call-site context every business (non-auth) operation requires. Callers
 * must always pass a token explicitly — the SDK never silently substitutes
 * an app-level token for a missing user-level one. */
export interface QdmpContext {
  accessToken: string;
}

// ---- auth ----

export interface AppTokenData {
  accessToken: string;
  expiresAt: string;
}

export interface SessionData {
  accessToken: string;
  refreshToken: string;
  expiresAt: string;
  openId: string;
}

export interface RefreshTokenData {
  accessToken: string;
  expiresAt?: string;
}

// ---- user ----

export interface UserMeData {
  id?: string;
  nickname?: string;
  avatar?: string;
  identityTags?: Array<Record<string, unknown>>;
  interestTags?: Array<Record<string, unknown>>;
}

// ---- island ----

export interface IslandInfo {
  id?: string;
  name?: string;
  image?: string;
  joined?: boolean;
}

export interface IslandDetailParams {
  id: string;
}

export interface IslandDetailData {
  island?: IslandInfo;
}

// ---- spu ----

export interface SpuInfo {
  id?: string;
  name?: string;
  image?: string;
  whiteBgPng?: string;
  typeId?: string;
  typeName?: string;
  wishCount?: string;
  markCount?: string;
  wishCount3day?: string;
  markCount3day?: string;
  entryProfileItems?: Array<Record<string, unknown>>;
}

export interface SpuDetailParams {
  id: string;
}

export interface SpuDetailData {
  spu?: SpuInfo;
}

export interface SpuSearchItem {
  id?: string;
  name?: string;
  image?: string;
  typeId?: string;
  typeName?: string;
  wishCount?: string;
  markCount?: string;
  [key: string]: unknown;
}

export interface SpuSearchParams {
  keyword?: string;
  ipTag?: string;
  typeId?: string;
  megaId?: string;
  classification?: string;
  filterOptions?: string[];
  onlyMega?: boolean;
  onlySpu?: boolean;
  offset?: string;
  limit?: string;
}

export interface SpuSearchData {
  items?: SpuSearchItem[];
  totalCount?: string;
}

// ---- tag ----

export interface TagInfo {
  id?: string;
  name?: string;
  image?: string;
  typeId?: string;
  typeName?: string;
  island?: IslandInfo | null;
}

export interface TagDetailParams {
  id: string;
}

export interface TagDetailData {
  tag?: TagInfo;
}

export interface TagSearchParams {
  keyword?: string;
  typeId?: string;
  classification?: string;
  filterOptions?: string[];
  offset?: string;
  limit?: string;
}

export interface TagSearchData {
  items?: TagInfo[];
  totalCount?: string;
}

// ---- marks ----

export interface MarkRating {
  value?: number;
}

export interface MarkSpuSummary {
  id?: string;
  name?: string;
  image?: string;
}

export interface MarkAddParams {
  spuId: string;
  rating?: MarkRating;
}

export interface MarkAddData {
  id?: string;
}

export interface MarkListItem {
  id?: string;
  spu?: MarkSpuSummary;
  markAt?: string;
  count?: string;
  rating?: MarkRating | null;
  createdAt?: string;
  typeId?: string;
}

export interface MarkListParams {
  limit: string;
  offset: string;
}

export interface MarkListData {
  items?: MarkListItem[];
  totalCount?: string;
}

export interface MarkSearchParams {
  typeId?: string;
  limit: string;
  offset: string;
}

export type MarkSearchData = MarkListData;

export interface MarkDetailParams {
  id: string;
  limit: string;
  offset: string;
}

export interface MarkHistoryItem {
  id?: string;
  markAt?: string;
  createdAt?: string;
}

export interface MarkDetailData {
  id?: string;
  spu?: MarkSpuSummary;
  markAt?: string;
  count?: string;
  rating?: MarkRating | null;
  createdAt?: string;
  typeId?: string;
  typeName?: string;
  marks?: MarkHistoryItem[];
  hasMore?: boolean;
}

// ---- wishspu ----

export type WishSpuType = 'WISH_SPU_TYPE_SPU';

export interface WishAddParams {
  ids: string[];
  type: WishSpuType;
}

export interface WishAddData {
  successCount?: string;
}

export interface WishCancelParams {
  ids: string[];
  type: WishSpuType;
}

export interface WishCancelData {
  successCount?: string;
}

export interface WishListItem {
  id?: string;
  spu?: MarkSpuSummary;
  typeId?: string;
  markAt?: string;
  createdAt?: string;
}

export interface WishListParams {
  typeId?: string;
  offset: string;
  limit: string;
}

export interface WishListData {
  items?: WishListItem[];
  totalCount?: string;
}

// ---- genai ----

export interface GenaiContentPart {
  text?: string;
  fileData?: {fileUri?: string; mimeType?: string};
  [key: string]: unknown;
}

export interface GenaiContent {
  role: string;
  parts: GenaiContentPart[];
}

export interface GenaiGenerateParams {
  model: string;
  contents: GenaiContent[];
  generationConfig?: Record<string, unknown>;
  safetySettings?: Array<Record<string, unknown>>;
  tools?: Array<Record<string, unknown>>;
}

export interface GenaiGenerateData {
  id?: string;
  status?: string;
}

export interface GenaiDetailParams {
  id: string;
}

export interface GenaiDetailData {
  id?: string;
  status?: 'PENDING' | 'RUNNING' | 'COMPLETED' | 'FAILED';
  response?: unknown;
}
