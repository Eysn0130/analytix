import { HUB_MODEL_PROVIDER_ID } from '../../shared/hub-account'
import { recordHubActivity } from '../hub-activity-observation'

recordHubActivity('moduleLoad')

let gatewayToken = ''

export function setHubGatewayRuntimeToken(token: string): void {
  gatewayToken = token.trim()
}

export function clearHubGatewayRuntimeToken(): void {
  gatewayToken = ''
}

export function hubGatewayRuntimeTokenForProvider(providerId: string): string {
  recordHubActivity('tokenRead')
  return providerId.trim() === HUB_MODEL_PROVIDER_ID ? gatewayToken : ''
}

export function hasHubGatewayRuntimeToken(): boolean {
  recordHubActivity('tokenRead')
  return Boolean(gatewayToken)
}
