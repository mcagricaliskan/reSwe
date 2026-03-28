export interface WSMessage {
  type: string
  task_id: number
  payload: Record<string, unknown>
}

type Callback = (msg: WSMessage) => void

class WebSocketClient {
  private ws: WebSocket | null = null
  private listeners = new Map<string, Set<Callback>>()
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private reconnectDelay = 1000

  connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${protocol}//${window.location.host}/ws`

    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      console.log('WebSocket connected')
      this.reconnectDelay = 1000
    }

    this.ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data)
        this.emit(msg.type, msg)
        this.emit('*', msg)
      } catch (e) {
        console.error('WS parse error:', e)
      }
    }

    this.ws.onclose = () => {
      console.log('WebSocket disconnected, reconnecting...')
      this.reconnectTimer = setTimeout(() => {
        this.reconnectDelay = Math.min(this.reconnectDelay * 2, 10000)
        this.connect()
      }, this.reconnectDelay)
    }

    this.ws.onerror = (err) => {
      console.error('WebSocket error:', err)
    }
  }

  disconnect() {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer)
    if (this.ws) this.ws.close()
  }

  on(type: string, callback: Callback): () => void {
    if (!this.listeners.has(type)) {
      this.listeners.set(type, new Set())
    }
    this.listeners.get(type)!.add(callback)
    return () => this.listeners.get(type)?.delete(callback)
  }

  private emit(type: string, data: WSMessage) {
    this.listeners.get(type)?.forEach(cb => cb(data))
  }
}

export const wsClient = new WebSocketClient()
