"use client";

import * as React from "react";
import { Camera, MousePointerClick } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuItemIcon,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type ConversationScreenshotActionsProps = {
  screenshotLatestLabel?: string;
  screenshotSelectLabel?: string;
  onScreenshotLatest?: () => void;
  onScreenshotSelect?: () => void;
};

type ConversationScreenshotMenuItemsProps = ConversationScreenshotActionsProps & {
  onCloseMenu?: () => void;
};

export function ConversationScreenshotMenuItems({
  screenshotLatestLabel,
  screenshotSelectLabel,
  onScreenshotLatest,
  onScreenshotSelect,
  onCloseMenu,
}: ConversationScreenshotMenuItemsProps) {
  return (
    <>
      <DropdownMenuItem
        disabled={!onScreenshotSelect}
        onSelect={(event) => {
          event.preventDefault();
          if (!onScreenshotSelect) {
            return;
          }
          onCloseMenu?.();
          onScreenshotSelect();
        }}
      >
        <DropdownMenuItemIcon icon={MousePointerClick} className="text-current" />
        {screenshotSelectLabel}
      </DropdownMenuItem>
      <DropdownMenuItem
        disabled={!onScreenshotLatest}
        onSelect={(event) => {
          event.preventDefault();
          if (!onScreenshotLatest) {
            return;
          }
          onCloseMenu?.();
          onScreenshotLatest();
        }}
      >
        <DropdownMenuItemIcon icon={Camera} className="text-current" />
        {screenshotLatestLabel}
      </DropdownMenuItem>
    </>
  );
}

type ConversationScreenshotIconDropdownProps = {
  label: string;
  className?: string;
} & ConversationScreenshotActionsProps;

export function ConversationScreenshotIconDropdown({
  label,
  className,
  screenshotLatestLabel,
  screenshotSelectLabel,
  onScreenshotLatest,
  onScreenshotSelect,
}: ConversationScreenshotIconDropdownProps) {
  const [open, setOpen] = React.useState(false);
  const hasAction = Boolean(onScreenshotLatest || onScreenshotSelect);

  return (
    <DropdownMenu modal={false} open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className={cn(
            "size-8 shrink-0 rounded-lg text-muted-foreground shadow-none hover:bg-muted hover:text-foreground",
            className,
          )}
          disabled={!hasAction}
          aria-label={label}
          title={label}
        >
          <Camera className="size-4 stroke-[1.8]" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" sideOffset={8} className="min-w-40">
        <ConversationScreenshotMenuItems
          screenshotLatestLabel={screenshotLatestLabel}
          screenshotSelectLabel={screenshotSelectLabel}
          onScreenshotLatest={onScreenshotLatest}
          onScreenshotSelect={onScreenshotSelect}
          onCloseMenu={() => setOpen(false)}
        />
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export function ConversationScreenshotSubmenu({
  label,
  screenshotLatestLabel,
  screenshotSelectLabel,
  onScreenshotLatest,
  onScreenshotSelect,
  onCloseMenu,
}: { label: string } & ConversationScreenshotMenuItemsProps) {
  const hasAction = Boolean(onScreenshotLatest || onScreenshotSelect);
  return (
    <DropdownMenuSub>
      <DropdownMenuSubTrigger disabled={!hasAction}>
        <DropdownMenuItemIcon icon={Camera} className="text-current" />
        {label}
      </DropdownMenuSubTrigger>
      <DropdownMenuSubContent className="min-w-40 p-1.5">
        <ConversationScreenshotMenuItems
          screenshotLatestLabel={screenshotLatestLabel}
          screenshotSelectLabel={screenshotSelectLabel}
          onScreenshotLatest={onScreenshotLatest}
          onScreenshotSelect={onScreenshotSelect}
          onCloseMenu={onCloseMenu}
        />
      </DropdownMenuSubContent>
    </DropdownMenuSub>
  );
}
