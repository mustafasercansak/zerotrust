import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "./locales/en.json";
import tr from "./locales/tr.json";

// Locale stored in user profile on the server (security signal — see useAuth).
// localStorage is only the *local* fallback until the profile loads.
const savedLocale = localStorage.getItem("locale") ?? "tr";

i18n.use(initReactI18next).init({
  // Each top-level key in the JSON files becomes an i18next namespace.
  resources: { en, tr },
  lng: savedLocale,
  fallbackLng: "tr",
  interpolation: {
    // React escapes by default; no need for i18next to double-escape.
    escapeValue: false,
  },
  // Disable i18next's built-in language detection (we drive locale from the API).
  detection: undefined,
});

export default i18n;
