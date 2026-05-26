"use client";

import * as Primitive from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import { cn } from "@/lib/utils";

export const Dialog = Primitive.Root;
export const DialogTrigger = Primitive.Trigger;
export const DialogClose = Primitive.Close;

export function DialogContent({
  className,
  children,
  showClose = true,
  ...props
}: React.ComponentPropsWithoutRef<typeof Primitive.Content> & { showClose?: boolean }) {
  return (
    <Primitive.Portal>
      <Primitive.Overlay className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm" />
      <Primitive.Content
        className={cn(
          "fixed left-1/2 top-1/2 z-50 -translate-x-1/2 -translate-y-1/2",
          "w-full max-w-md bg-gray-900 border border-gray-700 rounded-2xl p-6 shadow-2xl",
          "focus:outline-none",
          className,
        )}
        {...props}
      >
        {children}
        {showClose && (
          <Primitive.Close className="absolute right-4 top-4 p-1 rounded text-gray-500 hover:text-white hover:bg-gray-800 transition-colors">
            <X size={14} />
          </Primitive.Close>
        )}
      </Primitive.Content>
    </Primitive.Portal>
  );
}

export function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("mb-5", className)} {...props} />;
}

export function DialogTitle({
  className,
  ...props
}: React.ComponentPropsWithoutRef<typeof Primitive.Title>) {
  return (
    <Primitive.Title
      className={cn("text-lg font-semibold text-white", className)}
      {...props}
    />
  );
}

export function DialogDescription({
  className,
  ...props
}: React.ComponentPropsWithoutRef<typeof Primitive.Description>) {
  return (
    <Primitive.Description
      className={cn("text-sm text-gray-400 mt-1", className)}
      {...props}
    />
  );
}

export function DialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex gap-3 pt-2", className)} {...props} />;
}
