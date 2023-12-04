import axios, { AxiosHeaders, type AxiosResponse } from "axios";

export type HttpClientMethod = "get" | "post" | "patch";

export interface HttpClientRequest<RequestData> {
  data?: RequestData,
  headers?: Headers,
  method?: HttpClientMethod,
  queryParams?: URLSearchParams,
  url: string,
}

export class HttpClientError { constructor(public resp: AxiosResponse) {} }

export class HttpClient {
  async do<RequestData, ResponseData>(
    req: HttpClientRequest<RequestData>
  ): Promise<ResponseData> {
    const headers = new AxiosHeaders()
    req.headers?.forEach((v: string, k: string) => headers.set(k, v))
    const resp = await axios<ResponseData>({
      data: req.data,
      headers: headers,
      method: req.method,
      url: req.url + (req.queryParams ? '?' + req.queryParams.toString() : ''),
    });
    if (resp.status < 200 || resp.status > 299) throw new HttpClientError(resp)
    return resp.data
  }
}
