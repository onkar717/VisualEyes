import { useEffect, useRef, useState, useCallback } from 'react';

export type WSReadyState = 'connecting' | 'open' | 'closed';

interface UseWebSocketOptions {
  reconnectDelay?: number; // ms between reconnect attempts, default 3000
}

export function useWebSocket<T = unknown>(
  url: string,
  options: UseWebSocketOptions = {}
) {
  const { reconnectDelay = 3000 } = options;
  const [readyState, setReadyState] = useState<WSReadyState>('connecting');
  const [lastMessage, setLastMessage] = useState<T | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const unmounted = useRef(false);

  const connect = useCallback(() => {
    if (unmounted.current) return;

    const ws = new WebSocket(url);
    wsRef.current = ws;
    setReadyState('connecting');

    ws.onopen = () => {
      if (!unmounted.current) setReadyState('open');
    };

    ws.onmessage = (e: MessageEvent) => {
      if (unmounted.current) return;
      try {
        setLastMessage(JSON.parse(e.data) as T);
      } catch {
        // non-JSON frame (server ping text etc.)   ignore
      }
    };

    ws.onclose = () => {
      if (unmounted.current) return;
      setReadyState('closed');
      // Auto-reconnect after delay
      reconnectTimer.current = setTimeout(connect, reconnectDelay);
    };

    ws.onerror = () => {
      ws.close(); // triggers onclose → reconnect
    };
  }, [url, reconnectDelay]);

  useEffect(() => {
    unmounted.current = false;
    connect();

    return () => {
      unmounted.current = true;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      wsRef.current?.close();
    };
  }, [connect]);

  return { readyState, lastMessage };
}
