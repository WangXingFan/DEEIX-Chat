"use client";

import * as React from "react";
import { useTranslations } from "next-intl";
import { Plus, RefreshCw, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { SettingsPage, SettingsSection } from "@/shared/components/settings-layout";
import { useSettingsUpstreams } from "@/features/settings/hooks/use-settings-upstreams";

export function SettingsUpstreams() {
  const t = useTranslations("settings.upstreamsPage");
  const { upstreams, remoteModels, loading, addUpstream, discover, addModels, remove } = useSettingsUpstreams();
  const [name, setName] = React.useState("");
  const [baseURL, setBaseURL] = React.useState("");
  const [compatible, setCompatible] = React.useState("openai");
  const [apiKeys, setApiKeys] = React.useState("");
  const [modelInputs, setModelInputs] = React.useState<Record<number, string>>({});

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
            const discovered = remoteModels[upstream.id] ?? [];
            const input = modelInputs[upstream.id] ?? "";
            return <Card key={upstream.id}>
              <CardHeader className="flex-row items-center justify-between gap-3"><CardTitle className="text-sm">{upstream.name}</CardTitle><Button aria-label={t("delete")} onClick={() => window.confirm(t("confirmDelete")) && void remove(upstream.id)} size="icon-sm" variant="ghost"><Trash2 /></Button></CardHeader>
              <CardContent className="space-y-3 text-xs text-muted-foreground">
                <div>{upstream.baseURL} · {upstream.compatible}</div>
                <div className="flex gap-2"><Button onClick={() => void discover(upstream.id)} size="sm" variant="outline"><RefreshCw />{t("discover")}</Button></div>
                {discovered.length > 0 ? <div className="space-y-2"><div>{discovered.filter((item) => !item.alreadyBound).map((item) => item.upstreamModelName).join(", ")}</div><Input value={input} onChange={(event) => setModelInputs((current) => ({ ...current, [upstream.id]: event.target.value }))} placeholder={t("modelsPlaceholder")} /><Button disabled={!input.trim()} onClick={() => void addModels(upstream.id, input.split(",").map((item) => item.trim()).filter(Boolean))} size="sm"><Plus />{t("addModels")}</Button></div> : null}
              </CardContent>
            </Card>;
          })}
        </div>
      </SettingsSection>
    </SettingsPage>
  );
}
