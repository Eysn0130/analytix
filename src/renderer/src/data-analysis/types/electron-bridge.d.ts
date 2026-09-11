declare global {
  interface Window {
    electronBridge?: {
      showContextMenu?: (template: unknown[]) => Promise<{ id: string | null } | null | undefined>
    }
  }
}

export {}
