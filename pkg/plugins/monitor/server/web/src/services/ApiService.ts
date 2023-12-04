import type { ApiCatchUp } from "@/types/api";
import type { HttpClient, HttpClientRequest } from "../components/HttpClient";
import type { ConfigService } from "./ConfigService";

export class DisabledError {}

export class ApiService {

  constructor(
    private configService: ConfigService,
    private httpClient: HttpClient,
  ) {}

  catchUp(): Promise<ApiCatchUp> {
    return this.do<void, ApiCatchUp>({ url: "/catch-up" })
  }

  private async do<RequestData, ResponseData>(req: HttpClientRequest<RequestData>): Promise<ResponseData> {
    const config = await this.configService.get()
    if (!config.api.url) throw new DisabledError()
    for (const k in config.api.headers) {
      if (!req.headers) req.headers = new Headers()
      req.headers.set(k, config.api.headers[k])
    }
    for (const k in config.api.queryParams) {
      if (!req.queryParams) req.queryParams = new URLSearchParams()
      req.queryParams.set(k, config.api.queryParams[k])
    }
    req.url = config.api.url + req.url
    return this.httpClient.do<RequestData, ResponseData>(req)
  }

}