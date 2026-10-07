export type SubscriptionFormat = "sub" | "clash";

export function createSubscriptionUrl(
  apiBaseURL: string,
  pageURL: string,
  format: SubscriptionFormat,
  token: string,
): string {
  const base = new URL(apiBaseURL, pageURL);
  if (base.protocol !== "http:" && base.protocol !== "https:") {
    throw new Error("请先设置有效的面板地址");
  }
  const subscription = new URL(`open_api/${format}`, base);
  subscription.searchParams.set("token", token);
  return subscription.href;
}
