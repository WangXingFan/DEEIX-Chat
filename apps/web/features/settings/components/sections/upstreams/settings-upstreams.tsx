"use client";

import * as React from "react";
import { useTranslations } from "next-intl";
import { Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SettingsPage, SettingsSection } from "@/shared/components/settings-layout";
import { useSettingsUpstreams } from "@/features/settings/hooks/use-settings-upstreams";
import type { UserRemoteModelDTO, UserUpstreamDTO } from "@/shared/api/upstreams-types";

export function SettingsUpstreams() {
  const t = useTranslations("settings.upstreamsPage");
  const { upstreams, remoteModels, loading, addUpstream, editUpstream, discover, addModels, remove } = useSettingsUpstreams();
  const [name, setName] = React.useState("");
  const [baseURL, setBaseURL] = React.useState("");
  const [compatible, setCompatible] = React.useState("openai");
  const [apiKeys, setApiKeys] = React.useState("");
  const [modelInputs, setModelInputs] = React.useState<Record<number, string>>({});
  const [editingUpstream, setEditingUpstream] = React.useState<UserUpstreamDTO | null>(null);
  const [editName, setEditName] = React.useState("");
  const [editBaseURL, setEditBaseURL] = React.useState("");
  const [editCompatible, setEditCompatible] = React.useState("openai");
  const [editAPIKey, setEditAPIKey] = React.useState("");
  const [discoveringUpstreamID, setDiscoveringUpstreamID] = React.useState<number | null>(null);
  const [selectedModels, setSelectedModels] = React.useState<Record<number, string[]>>({});
  const [deletingUpstream, setDeletingUpstream] = React.useState<UserUpstreamDTO | null>(null);

  function openEdit(upstream: UserUpstreamDTO) {
    setEditingUpstream(upstream);
    setEditName(upstream.name);
    setEditBaseURL(upstream.baseURL);
    setEditCompatible(upstream.compatible);
    setEditAPIKey("");
  }

  async function submitEdit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editingUpstream) return;
    const input: { name: string; baseURL: string; compatible: string; apiKeys?: string } = {
      name: editName,
      baseURL: editBaseURL,
      compatible: editCompatible,
    };
    if (editAPIKey.trim()) {
      input.apiKeys = JSON.stringify({ strategy: "failover", keys: [{ key: editAPIKey.trim(), status: "active" }] });
    }
    await editUpstream(editingUpstream.id, input);
    setEditingUpstream(null);
  }

  async function openDiscover(upstreamID: number) {
    setDiscoveringUpstreamID(upstreamID);
    const items = await discover(upstreamID);
    setSelectedModels((current) => ({
      ...current,
      [upstreamID]: items.filter((item) => !item.alreadyBound).map((item) => item.upstreamModelName),
    }));
  }

  function toggleModel(upstreamID: number, model: UserRemoteModelDTO, checked: boolean) {
    if (model.alreadyBound) return;
    setSelectedModels((current) => {
      const selected = new Set(current[upstreamID] ?? []);
      if (checked) selected.add(model.upstreamModelName);
      else selected.delete(model.upstreamModelName);
      return { ...current, [upstreamID]: Array.from(selected) };
    });
  }

  async function importSelectedModels() {
    if (discoveringUpstreamID === null) return;
    const names = selectedModels[discoveringUpstreamID] ?? [];
    if (names.length === 0) return;
    await addModels(discoveringUpstreamID, names);
    setDiscoveringUpstreamID(null);
  }

  async function confirmRemove() {
    if (!deletingUpstream) return;
    await remove(deletingUpstream.id);
    setDeletingUpstream(null);
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await addUpstream({ name, baseURL, compatible, apiKeys: JSON.stringify({ strategy: "failover", keys: [{ key: apiKeys, status: "active" }] }) });
    setName("");
    setBaseURL("");
    setApiKeys("");
  }

  return (
    <SettingsPage>
      <SettingsSection title={t("title")}>
        <p className="text-sm text-muted-foreground">{t("description")}</p>
        <Card>
          <CardHeader><CardTitle className="text-sm">{t("addTitle")}</CardTitle></CardHeader>
          <CardContent>
            <form className="grid gap-3 md:grid-cols-2" onSubmit={submit}>
              <Input value={name} onChange={(event) => setName(event.target.value)} placeholder={t("name")} required />
              <Input value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder={t("baseURL")} type="url" required />
              <Input value={compatible} onChange={(event) => setCompatible(event.target.value)} placeholder={t("compatible")} required />
              <Input value={apiKeys} onChange={(event) => setApiKeys(event.target.value)} placeholder={t("apiKey")} type="password" required />
              <Button className="md:col-span-2" type="submit"><Plus />{t("add")}</Button>
            </form>
          </CardContent>
        </Card>
      </SettingsSection>
      <SettingsSection title={t("yourUpstreams")}>
        {loading ? <p className="text-sm text-muted-foreground">{t("loading")}</p> : null}
        {!loading && upstreams.length === 0 ? <p className="text-sm text-muted-foreground">{t("empty")}</p> : null}
        <div className="grid gap-4">
          {upstreams.map((upstream) => {
            const input = modelInputs[upstream.id] ?? "";
            return <Card key={upstream.id}>
              <CardHeader className="flex-row items-center justify-between gap-3"><CardTitle className="text-sm">{upstream.name}</CardTitle><div className="flex items-center gap-1"><Button aria-label={t("edit")} onClick={() => openEdit(upstream)} size="icon-sm" variant="ghost"><Pencil /></Button><Button aria-label={t("delete")} onClick={() => setDeletingUpstream(upstream)} size="icon-sm" variant="ghost"><Trash2 /></Button></div></CardHeader>
              <CardContent className="space-y-3 text-xs text-muted-foreground">
                <div>{upstream.baseURL} · {upstream.compatible}</div>
                <div className="flex gap-2"><Button onClick={() => void openDiscover(upstream.id)} size="sm" variant="outline"><RefreshCw />{t("discover")}</Button></div>
                <div className="space-y-2"><Input value={input} onChange={(event) => setModelInputs((current) => ({ ...current, [upstream.id]: event.target.value }))} placeholder={t("modelsPlaceholder")} /><Button disabled={!input.trim()} onClick={() => void addModels(upstream.id, input.split(",").map((item) => item.trim()).filter(Boolean))} size="sm"><Plus />{t("addModels")}</Button></div>
              </CardContent>
            </Card>;
          })}
        </div>
      </SettingsSection>
      <AlertDialog open={deletingUpstream !== null} onOpenChange={(open) => !open && setDeletingUpstream(null)}>
        <AlertDialogContent size="compact">
          <AlertDialogHeader><AlertDialogTitle>{t("deleteTitle")}</AlertDialogTitle><AlertDialogDescription>{t("confirmDelete")}</AlertDialogDescription></AlertDialogHeader>
          <AlertDialogFooter><AlertDialogCancel>{t("cancel")}</AlertDialogCancel><AlertDialogAction variant="destructive" onClick={() => void confirmRemove()}>{t("delete")}</AlertDialogAction></AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <Dialog open={editingUpstream !== null} onOpenChange={(open) => !open && setEditingUpstream(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("editTitle")}</DialogTitle><DialogDescription>{t("editDescription")}</DialogDescription></DialogHeader>
          <form className="grid gap-3" onSubmit={submitEdit}>
            <Input value={editName} onChange={(event) => setEditName(event.target.value)} placeholder={t("name")} required />
            <Input value={editBaseURL} onChange={(event) => setEditBaseURL(event.target.value)} placeholder={t("baseURL")} type="url" required />
            <Input value={editCompatible} onChange={(event) => setEditCompatible(event.target.value)} placeholder={t("compatible")} required />
            <Input value={editAPIKey} onChange={(event) => setEditAPIKey(event.target.value)} placeholder={t("apiKeyOptional")} type="password" />
            <DialogFooter><Button type="submit">{t("save")}</Button></DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog open={discoveringUpstreamID !== null} onOpenChange={(open) => !open && setDiscoveringUpstreamID(null)}>
        <DialogContent>
          <DialogHeader><DialogTitle>{t("discoverTitle")}</DialogTitle><DialogDescription>{t("discoverDescription")}</DialogDescription></DialogHeader>
          <div className="max-h-[min(55vh,420px)] space-y-2 overflow-y-auto">
            {(discoveringUpstreamID === null ? [] : remoteModels[discoveringUpstreamID] ?? []).map((model) => {
              const checked = model.alreadyBound || (selectedModels[discoveringUpstreamID ?? 0] ?? []).includes(model.upstreamModelName);
              return <label className="flex cursor-pointer items-center gap-3 rounded-md border border-border/60 px-3 py-2 text-sm" key={model.upstreamModelName}>
                <Checkbox checked={checked} disabled={model.alreadyBound} onCheckedChange={(value) => toggleModel(discoveringUpstreamID ?? 0, model, value === true)} />
                <span className="min-w-0 flex-1 truncate">{model.upstreamModelName}</span>
                {model.alreadyBound ? <span className="text-xs text-muted-foreground">{t("added")}</span> : null}
              </label>;
            })}
          </div>
          <DialogFooter><Button disabled={(selectedModels[discoveringUpstreamID ?? 0] ?? []).length === 0} onClick={() => void importSelectedModels()}><Plus />{t("addSelected")}</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </SettingsPage>
  );
}
