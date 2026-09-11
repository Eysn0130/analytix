import type { AnalytixApi } from '../shared/analytix-api'

export type * from '../shared/analytix-api'

declare global {
  interface Window {
    analytix: AnalytixApi
  }
}
