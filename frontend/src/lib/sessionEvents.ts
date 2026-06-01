export function listenToSessionEvents(onRevoked: () => void, onChange?: () => void): () => void {
  if (typeof EventSource === "undefined") {
    return () => {};
  }

  const es = new EventSource("/api/v1/sessions/events");

  es.onmessage = (event) => {
    if (event.data === "revoked") {
      onRevoked();
    } else if (event.data === "change") {
      onChange?.();
    }
  };

  return () => {
    es.close();
  };
}
