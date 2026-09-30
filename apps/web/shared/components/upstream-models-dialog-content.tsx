"use client";

import type * as React from "react";
import { DialogContent, DialogHeightTransition } from "@/components/ui/dialog";

export function UpstreamModelsDialogContent({
  children,
}: Pick<React.ComponentProps<typeof DialogContent>, "children">) {
  return (
    <DialogContent className="w-[calc(100vw-2rem)] gap-0 overflow-hidden p-0 md:w-[calc(100vw-8rem)] sm:max-w-[860px]">
      <DialogHeightTransition contentClassName="max-h-[min(86vh,760px)]">
        {children}
      </DialogHeightTransition>
    </DialogContent>
  );
}
