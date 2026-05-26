import { redirect } from "next/navigation";
import { useLocale } from "next-intl";

export default function RootPage() {
  const locale = useLocale();
  redirect(`/${locale}/auth/login`);
}
