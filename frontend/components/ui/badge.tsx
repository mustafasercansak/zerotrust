import { cn } from "@/lib/utils";

interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: "default" | "success" | "danger" | "indigo" | "sky" | "muted";
}

export function Badge({ className, variant = "default", ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center text-xs px-2 py-0.5 rounded-full border font-medium",
        variant === "default" && "bg-gray-800 text-gray-300 border-gray-700",
        variant === "success" && "bg-green-900/40 text-green-400 border-green-700",
        variant === "danger" && "bg-red-900/40 text-red-400 border-red-700",
        variant === "indigo" && "bg-indigo-900/60 text-indigo-300 border-indigo-700",
        variant === "sky" && "bg-sky-900/50 text-sky-300 border-sky-800",
        variant === "muted" && "text-gray-500 border-transparent",
        className,
      )}
      {...props}
    />
  );
}
