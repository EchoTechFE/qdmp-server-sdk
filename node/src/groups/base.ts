import type {HttpClient} from '../http.js';

/** Shared dependencies every business group needs: the transport and the
 * client-level config that doesn't vary per call (appId for the genai
 * scheme's x-openapi-app-id header, qdmpVersion for the standard scheme's
 * x-echo-qdmp-version header). */
export interface GroupDeps {
  http: HttpClient;
  appId: string;
  qdmpVersion: string;
}
