export type ProductEdition = "standard" | "professional";

export const DEFAULT_PRODUCT_EDITION: ProductEdition = "standard";

const PRODUCT_EDITION_ALIASES: Record<string, ProductEdition> = {
  standard: "standard",
  normal: "standard",
  basic: "standard",
  personal: "standard",
  "普通": "standard",
  "普通版": "standard",
  professional: "professional",
  pro: "professional",
  business: "professional",
  enterprise: "professional",
  "专业": "professional",
  "专业版": "professional"
};

export function normalizeProductEdition(value: unknown): ProductEdition {
  const normalized = String(value || "").trim().toLowerCase();
  return PRODUCT_EDITION_ALIASES[normalized] || DEFAULT_PRODUCT_EDITION;
}

export function defaultRouteForProductEdition(edition: ProductEdition): string {
  return edition === "standard" ? "/cases" : "/";
}
