import { useEffect, useRef, useState, useCallback } from "react";
import { api } from "@/lib/api";

// How many seconds before expiry to show the warning dialog.
const WARN_BEFORE_SECONDS = 60;

// Activity events that reset the idle timer.
const ACTIVITY_EVENTS = ["mousemove", "mousedown", "keydown", "touchstart", "scroll", "click"] as const;

export interface IdleTimeoutState {
  warningVisible: boolean;
  secondsRemaining: number;
  extendSession: () => void;
  dismissWarning: () => void;
}

export function useIdleTimeout(onExpire: () => void): IdleTimeoutState {
  const [warningVisible, setWarningVisible] = useState(false);
  const [secondsRemaining, setSecondsRemaining] = useState(WARN_BEFORE_SECONDS);

  const idleTimeoutRef = useRef<number>(300); // default: 5 min
  const lastActivityRef = useRef<number>(Date.now());
  const warningVisibleRef = useRef(false);
  const onExpireRef = useRef(onExpire);
  onExpireRef.current = onExpire;

  // Fetch idle timeout once on mount.
  useEffect(() => {
    api.getSessionPolicy()
      .then((policy) => { idleTimeoutRef.current = policy.idle_timeout_seconds; })
      .catch(() => { /* keep default */ });
  }, []);

  const resetActivity = useCallback(() => {
    lastActivityRef.current = Date.now();
    if (warningVisibleRef.current) {
      warningVisibleRef.current = false;
      setWarningVisible(false);
      setSecondsRemaining(WARN_BEFORE_SECONDS);
    }
  }, []);

  const extendSession = useCallback(() => {
    resetActivity();
  }, [resetActivity]);

  const dismissWarning = useCallback(() => {
    resetActivity();
  }, [resetActivity]);

  // Register activity listeners.
  useEffect(() => {
    ACTIVITY_EVENTS.forEach((e) => window.addEventListener(e, resetActivity, { passive: true }));
    return () => {
      ACTIVITY_EVENTS.forEach((e) => window.removeEventListener(e, resetActivity));
    };
  }, [resetActivity]);

  // Tick every second to check idle state and drive the countdown.
  useEffect(() => {
    const interval = setInterval(() => {
      const idleSeconds = (Date.now() - lastActivityRef.current) / 1000;
      const timeoutSeconds = idleTimeoutRef.current;
      const remaining = Math.max(0, timeoutSeconds - idleSeconds);

      if (remaining <= 0) {
        clearInterval(interval);
        setWarningVisible(false);
        warningVisibleRef.current = false;
        onExpireRef.current();
        return;
      }

      if (remaining <= WARN_BEFORE_SECONDS) {
        if (!warningVisibleRef.current) {
          warningVisibleRef.current = true;
          setWarningVisible(true);
        }
        setSecondsRemaining(Math.ceil(remaining));
      }
    }, 1000);

    return () => clearInterval(interval);
  }, []);

  return { warningVisible, secondsRemaining, extendSession, dismissWarning };
}
