import { useRef, useState } from "react";
import { api, ApiError } from "@/lib/api";

interface PendingAction {
  action: () => Promise<unknown>;
  resolve: (v: unknown) => void;
  reject: (e: unknown) => void;
}

export function useStepUp() {
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const pendingRef = useRef<PendingAction | null>(null);

  async function runWithStepUp<T>(action: () => Promise<T>): Promise<T> {
    try {
      return await action();
    } catch (err) {
      if (!(err instanceof ApiError) || err.message !== "mfa_required") throw err;
      return new Promise<T>((resolve, reject) => {
        pendingRef.current = {
          action: action as () => Promise<unknown>,
          resolve: resolve as (v: unknown) => void,
          reject,
        };
        setOpen(true);
      });
    }
  }

  async function handleSubmit(code: string) {
    const pending = pendingRef.current;
    if (!pending) return;
    setSubmitting(true);
    setError("");
    try {
      await api.mfaStepUp(code);
      const result = await pending.action();
      pending.resolve(result);
      pendingRef.current = null;
      setOpen(false);
    } catch (err) {
      if (err instanceof ApiError && (err.message === "mfa_required" || err.message === "too_many_attempts")) {
        setError(err.message);
      } else {
        pending.reject(err);
        pendingRef.current = null;
        setOpen(false);
        setError("");
      }
    } finally {
      setSubmitting(false);
    }
  }

  function handleClose() {
    const pending = pendingRef.current;
    if (pending) {
      pending.reject(new ApiError("mfa_required", 403));
    }
    pendingRef.current = null;
    setOpen(false);
    setError("");
  }

  return { runWithStepUp, stepUpOpen: open, stepUpError: error, stepUpSubmitting: submitting, handleStepUpSubmit: handleSubmit, handleStepUpClose: handleClose };
}
