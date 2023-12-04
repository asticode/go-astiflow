import { ApiService } from "./ApiService";
import { FlowService } from "./FlowService";
import { HttpClient } from "../components/HttpClient";
import { LoggerService } from "./LoggerService";
import { PushService } from "./PushService";
import { StoreService } from "./StoreService";
import { ReplayService } from "./ReplayService";
import { ConfigService } from "./ConfigService";

const storeService = new StoreService()
const loggerService = new LoggerService()
const httpClient = new HttpClient()
const configService = new ConfigService(httpClient)
const apiService = new ApiService(configService, httpClient)
const pushService = new PushService(configService, loggerService, storeService)
const flowService = new FlowService(apiService, pushService, storeService)
const replayService = new ReplayService(flowService, loggerService, storeService)

export {
    flowService,
    loggerService,
    replayService,
    storeService,
}