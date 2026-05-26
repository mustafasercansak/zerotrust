import { cn } from "@/lib/utils";

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "secondary" | "ghost" | "danger";
  size?: "sm" | "md" | "icon";
}

export function Button({
  className,
  variant = "primary",
  size = "md",
  ...props
}: ButtonProps) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-1.5 font-medium transition-colors rounded-lg",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        variant === "primary" && "bg-indigo-600 hover:bg-indigo-500 text-white",
        variant === "secondary" && "bg-gray-800 hover:bg-gray-700 text-gray-300 border border-gray-700 hover:border-gray-500",
        variant === "ghost" && "text-gray-400 hover:text-white hover:bg-gray-800",
        variant === "danger" && "text-red-400 hover:text-red-300 hover:bg-red-900/20",
        size === "sm" && "text-xs px-3 py-1.5",
        size === "md" && "text-sm px-4 py-2.5",
        size === "icon" && "p-1.5",
        className,
      )}
      {...props}
    />
  );
}
