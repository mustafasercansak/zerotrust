import type { Metadata } from "next";
import { NextIntlClientProvider } from "next-intl";
import { getMessages } from "next-intl/server";
import TokenRefreshProvider from "@/components/TokenRefreshProvider";
import "../globals.css";

export const metadata: Metadata = {
  title: "ZeroTrust",
  description: "Zero Trust Security Platform",
};

export default async function LocaleLayout({
  children,
  params,
}: {
  children: React.ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  const messages = await getMessages();

  return (
    <html lang={locale}>
      <body className="font-sans antialiased">
        <NextIntlClientProvider messages={messages}>
          <TokenRefreshProvider>
            {children}
          </TokenRefreshProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
