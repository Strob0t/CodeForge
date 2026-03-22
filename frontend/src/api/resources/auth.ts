import type { CoreClient } from "../core";
import { url } from "../factory";
import type {
  APIKeyInfo,
  ChangePasswordRequest,
  CreateAPIKeyRequest,
  CreateAPIKeyResponse,
  CreateVCSAccountRequest,
  DeviceFlowResponse,
  ForgotPasswordRequest,
  InitialSetupRequest,
  LoginRequest,
  LoginResponse,
  ProviderStatusResponse,
  ResetPasswordRequest,
  SetupStatusResponse,
  SubscriptionProvidersResponse,
  User,
  VCSAccount,
} from "../types";

export function createAuthResource(c: CoreClient) {
  return {
    login: (data: LoginRequest) => c.post<LoginResponse>("/auth/login", data),

    refresh: () => c.post<LoginResponse>("/auth/refresh"),

    logout: () => c.post<{ status: string }>("/auth/logout"),

    me: () => c.get<User>("/auth/me"),

    changePassword: (data: ChangePasswordRequest) =>
      c.post<{ status: string }>("/auth/change-password", data),

    setupStatus: () => c.get<SetupStatusResponse>("/auth/setup-status"),

    setup: (data: InitialSetupRequest) => c.post<LoginResponse>("/auth/setup", data),

    forgotPassword: (data: ForgotPasswordRequest) =>
      c.post<{ status: string }>("/auth/forgot-password", data),

    resetPassword: (data: ResetPasswordRequest) =>
      c.post<{ status: string }>("/auth/reset-password", data),

    createAPIKey: (data: CreateAPIKeyRequest) =>
      c.post<CreateAPIKeyResponse>("/auth/api-keys", data),

    listAPIKeys: () => c.get<APIKeyInfo[]>("/auth/api-keys"),

    deleteAPIKey: (id: string) => c.del<undefined>(url`/auth/api-keys/${id}`),

    githubOAuth: () => c.get<{ url: string; state: string }>("/auth/github"),
  };
}

export function createVCSAccountsResource(c: CoreClient) {
  return {
    list: () => c.get<VCSAccount[]>("/vcs-accounts"),

    create: (data: CreateVCSAccountRequest) => c.post<VCSAccount>("/vcs-accounts", data),

    delete: (id: string) => c.del<undefined>(url`/vcs-accounts/${id}`),

    test: (id: string) => c.post<{ status: string }>(url`/vcs-accounts/${id}/test`),
  };
}

export function createSubscriptionProvidersResource(c: CoreClient) {
  return {
    list: () => c.get<SubscriptionProvidersResponse>("/auth/providers"),

    connect: (provider: string) =>
      c.post<DeviceFlowResponse>(`/auth/providers/${encodeURIComponent(provider)}/connect`),

    status: (provider: string) =>
      c.get<ProviderStatusResponse>(`/auth/providers/${encodeURIComponent(provider)}/status`),

    disconnect: (provider: string) =>
      c.del<undefined>(`/auth/providers/${encodeURIComponent(provider)}/disconnect`),
  };
}
