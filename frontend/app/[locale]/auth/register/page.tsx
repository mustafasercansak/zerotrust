import { redirect } from "next/navigation";

export default function RegisterPage({
  params,
}: {
  params: { locale: string };
}) {
  redirect(`/${params.locale}/auth/login`);
}
