import { cn } from "@/lib/utils";

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "w-full rounded-lg bg-gray-800 border border-gray-700 text-white px-4 py-2.5 text-sm",
        "placeholder:text-gray-500",
        "focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-transparent",
        "disabled:opacity-50 disabled:cursor-not-allowed",
        className,
      )}
      {...props}
    />
  );
}

export function Label({ className, ...props }: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn("block text-xs text-gray-400 mb-1", className)}
      {...props}
    />
  );
}
