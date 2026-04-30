import { useMemo, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  Clipboard,
  Loader2,
  Plus,
  RefreshCw,
  Send,
  ShieldCheck,
  Trash2
} from "lucide-react";
import { createBucket } from "./api";
import type { APIErrorResponse, BucketRequestPayload, BucketResult, EncryptionMode, Environment } from "./types";

interface ExtraTag {
  id: number;
  key: string;
  value: string;
}

interface FormState {
  team: string;
  environment: Environment;
  bucketName: string;
  enableVersioning: boolean;
  encryption: EncryptionMode;
  kmsKeyArn: string;
  costCenter: string;
  dataClass: string;
  businessOwner: string;
  purpose: string;
  tags: ExtraTag[];
}

const initialForm: FormState = {
  team: "payments",
  environment: "dev",
  bucketName: "",
  enableVersioning: true,
  encryption: "AES256",
  kmsKeyArn: "",
  costCenter: "payments",
  dataClass: "internal",
  businessOwner: "",
  purpose: "application-storage",
  tags: []
};

const teamPattern = /^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$/;

function App() {
  const [form, setForm] = useState<FormState>(initialForm);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [result, setResult] = useState<BucketResult | null>(null);
  const [error, setError] = useState<(APIErrorResponse & { fallback?: string }) | null>(null);
  const [copied, setCopied] = useState<string | null>(null);

  const validation = useMemo(() => validateForm(form), [form]);
  const payload = useMemo(() => buildPayload(form), [form]);

  const updateField = <Key extends keyof FormState>(key: Key, value: FormState[Key]) => {
    setForm((current) => ({ ...current, [key]: value }));
    setError(null);
  };

  const addTag = () => {
    setForm((current) => ({
      ...current,
      tags: [...current.tags, { id: Date.now(), key: "", value: "" }]
    }));
  };

  const updateTag = (id: number, key: keyof ExtraTag, value: string) => {
    setForm((current) => ({
      ...current,
      tags: current.tags.map((tag) => (tag.id === id ? { ...tag, [key]: value } : tag))
    }));
  };

  const removeTag = (id: number) => {
    setForm((current) => ({
      ...current,
      tags: current.tags.filter((tag) => tag.id !== id)
    }));
  };

  const reset = () => {
    setForm(initialForm);
    setResult(null);
    setError(null);
    setCopied(null);
  };

  const copyText = async (label: string, value: string) => {
    await navigator.clipboard.writeText(value);
    setCopied(label);
    window.setTimeout(() => setCopied(null), 1800);
  };

  const submit = async () => {
    if (!validation.valid || isSubmitting) {
      return;
    }

    setIsSubmitting(true);
    setResult(null);
    setError(null);

    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 30000);

    try {
      const response = await createBucket(payload, controller.signal);
      setResult(response);
    } catch (caught) {
      const requestError = caught as Error & { details?: APIErrorResponse };
      setError(requestError.details ?? { error: "request_failed", message: requestError.message, fallback: requestError.message });
    } finally {
      window.clearTimeout(timeout);
      setIsSubmitting(false);
    }
  };

  return (
    <main className="app-shell">
      <section className="workspace">
        <header className="topbar">
          <div>
            <p className="eyebrow">Platform self-service</p>
            <h1>Managed S3 bucket request</h1>
          </div>
          <div className="status-pill">
            <ShieldCheck size={16} aria-hidden="true" />
            Guarded AWS provisioning
          </div>
        </header>

        <div className="content-grid">
          <form className="request-panel" onSubmit={(event) => event.preventDefault()}>
            <section className="panel-section">
              <div className="section-title">
                <span>Requester</span>
                <span className="section-index">01</span>
              </div>
              <div className="field-grid two">
                <label className="field">
                  <span>Team</span>
                  <input
                    value={form.team}
                    onChange={(event) => updateField("team", event.target.value.toLowerCase())}
                    placeholder="payments"
                    autoComplete="organization"
                  />
                  {validation.fields.team && <small className="field-error">{validation.fields.team}</small>}
                </label>
                <label className="field">
                  <span>Cost center</span>
                  <input
                    value={form.costCenter}
                    onChange={(event) => updateField("costCenter", event.target.value)}
                    placeholder="payments"
                  />
                </label>
              </div>
              <div className="field">
                <span>Environment</span>
                <div className="segmented" role="group" aria-label="Environment">
                  {(["dev", "stage", "prod"] as Environment[]).map((environment) => (
                    <button
                      key={environment}
                      type="button"
                      className={form.environment === environment ? "active" : ""}
                      onClick={() => updateField("environment", environment)}
                    >
                      {environment}
                    </button>
                  ))}
                </div>
              </div>
            </section>

            <section className="panel-section">
              <div className="section-title">
                <span>Bucket settings</span>
                <span className="section-index">02</span>
              </div>
              <label className="field">
                <span>Bucket name</span>
                <input
                  value={form.bucketName}
                  onChange={(event) => updateField("bucketName", event.target.value.toLowerCase())}
                  placeholder="generated by platform"
                />
              </label>
              <div className="field-grid two">
                <label className="toggle-row">
                  <input
                    type="checkbox"
                    checked={form.enableVersioning}
                    onChange={(event) => updateField("enableVersioning", event.target.checked)}
                  />
                  <span>
                    <strong>Versioning</strong>
                    <small>{form.enableVersioning ? "Enabled" : "Disabled"}</small>
                  </span>
                </label>
                <label className="field">
                  <span>Data classification</span>
                  <select value={form.dataClass} onChange={(event) => updateField("dataClass", event.target.value)}>
                    <option value="public">public</option>
                    <option value="internal">internal</option>
                    <option value="confidential">confidential</option>
                    <option value="restricted">restricted</option>
                  </select>
                </label>
              </div>
              <div className="field">
                <span>Encryption</span>
                <div className="segmented" role="group" aria-label="Encryption">
                  {(["AES256", "aws:kms"] as EncryptionMode[]).map((encryption) => (
                    <button
                      key={encryption}
                      type="button"
                      className={form.encryption === encryption ? "active" : ""}
                      onClick={() => updateField("encryption", encryption)}
                    >
                      {encryption}
                    </button>
                  ))}
                </div>
              </div>
              {form.encryption === "aws:kms" && (
                <label className="field">
                  <span>KMS key ARN</span>
                  <input
                    value={form.kmsKeyArn}
                    onChange={(event) => updateField("kmsKeyArn", event.target.value)}
                    placeholder="arn:aws:kms:us-east-1:123456789012:key/..."
                  />
                  {validation.fields.kmsKeyArn && <small className="field-error">{validation.fields.kmsKeyArn}</small>}
                </label>
              )}
            </section>

            <section className="panel-section">
              <div className="section-title">
                <span>Business context</span>
                <span className="section-index">03</span>
              </div>
              <div className="field-grid two">
                <label className="field">
                  <span>Business owner</span>
                  <input
                    value={form.businessOwner}
                    onChange={(event) => updateField("businessOwner", event.target.value)}
                    placeholder="owner name"
                  />
                </label>
                <label className="field">
                  <span>Purpose</span>
                  <input
                    value={form.purpose}
                    onChange={(event) => updateField("purpose", event.target.value)}
                    placeholder="application-storage"
                  />
                </label>
              </div>
              <div className="tag-header">
                <span>Additional tags</span>
                <button type="button" className="icon-button text-button" onClick={addTag}>
                  <Plus size={16} aria-hidden="true" />
                  Add tag
                </button>
              </div>
              {form.tags.length > 0 && (
                <div className="tag-list">
                  {form.tags.map((tag) => (
                    <div className="tag-row" key={tag.id}>
                      <input value={tag.key} onChange={(event) => updateTag(tag.id, "key", event.target.value)} placeholder="Key" />
                      <input value={tag.value} onChange={(event) => updateTag(tag.id, "value", event.target.value)} placeholder="Value" />
                      <button type="button" className="icon-button" onClick={() => removeTag(tag.id)} aria-label="Remove tag">
                        <Trash2 size={16} aria-hidden="true" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </form>

          <aside className="review-panel">
            <section className="summary-block">
              <div className="section-title">
                <span>Request summary</span>
                <span className="section-index">Ready</span>
              </div>
              <dl className="summary-list">
                <div>
                  <dt>Team</dt>
                  <dd>{payload.team || "not set"}</dd>
                </div>
                <div>
                  <dt>Environment</dt>
                  <dd>{payload.environment}</dd>
                </div>
                <div>
                  <dt>Versioning</dt>
                  <dd>{payload.enableVersioning ? "enabled" : "disabled"}</dd>
                </div>
                <div>
                  <dt>Encryption</dt>
                  <dd>{payload.encryption}</dd>
                </div>
              </dl>
              <pre className="payload-preview">{JSON.stringify(payload, null, 2)}</pre>
            </section>

            {error && (
              <section className="message error-message">
                <AlertCircle size={18} aria-hidden="true" />
                <div>
                  <strong>{error.error}</strong>
                  <p>{error.message || error.fallback}</p>
                  {error.fields && (
                    <ul>
                      {Object.entries(error.fields).map(([field, message]) => (
                        <li key={field}>
                          {field}: {message}
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </section>
            )}

            {result && (
              <section className="message success-message">
                <CheckCircle2 size={18} aria-hidden="true" />
                <div>
                  <strong>Bucket created</strong>
                  <dl className="result-list">
                    <div>
                      <dt>Name</dt>
                      <dd>
                        <span>{result.bucketName}</span>
                        <button type="button" className="icon-button" onClick={() => copyText("bucket", result.bucketName)} aria-label="Copy bucket name">
                          <Clipboard size={15} aria-hidden="true" />
                        </button>
                      </dd>
                    </div>
                    <div>
                      <dt>ARN</dt>
                      <dd>
                        <span>{result.bucketArn}</span>
                        <button type="button" className="icon-button" onClick={() => copyText("arn", result.bucketArn)} aria-label="Copy bucket ARN">
                          <Clipboard size={15} aria-hidden="true" />
                        </button>
                      </dd>
                    </div>
                    <div>
                      <dt>Region</dt>
                      <dd>{result.region}</dd>
                    </div>
                  </dl>
                  {copied && <p className="copy-state">Copied {copied}</p>}
                </div>
              </section>
            )}

            <div className="actions">
              <button type="button" className="secondary-button" onClick={reset}>
                <RefreshCw size={16} aria-hidden="true" />
                Reset
              </button>
              <button type="button" className="primary-button" disabled={!validation.valid || isSubmitting} onClick={submit}>
                {isSubmitting ? <Loader2 className="spin" size={17} aria-hidden="true" /> : <Send size={17} aria-hidden="true" />}
                Create bucket
              </button>
            </div>
          </aside>
        </div>
      </section>
    </main>
  );
}

function validateForm(form: FormState) {
  const fields: Record<string, string> = {};

  if (!teamPattern.test(form.team.trim())) {
    fields.team = "Use 3-40 lowercase letters, numbers, or hyphens.";
  }

  if (form.encryption === "aws:kms" && form.kmsKeyArn.trim() === "") {
    fields.kmsKeyArn = "KMS key ARN is required for aws:kms.";
  }

  for (const tag of form.tags) {
    if ((tag.key.trim() === "" && tag.value.trim() !== "") || tag.key.length > 128 || tag.value.length > 256) {
      fields.tags = "Tag keys are required and must stay within AWS tag limits.";
      break;
    }
  }

  return {
    valid: Object.keys(fields).length === 0,
    fields
  };
}

function buildPayload(form: FormState): BucketRequestPayload {
  const tags = Object.fromEntries(
    [
      ["CostCenter", form.costCenter],
      ["DataClass", form.dataClass],
      ["BusinessOwner", form.businessOwner],
      ["Purpose", form.purpose],
      ...form.tags.map((tag) => [tag.key, tag.value])
    ]
      .map(([key, value]) => [key.trim(), value.trim()])
      .filter(([key, value]) => key !== "" && value !== "")
  );

  return {
    team: form.team.trim(),
    environment: form.environment,
    ...(form.bucketName.trim() ? { bucketName: form.bucketName.trim() } : {}),
    enableVersioning: form.enableVersioning,
    encryption: form.encryption,
    ...(form.encryption === "aws:kms" ? { kmsKeyArn: form.kmsKeyArn.trim() } : {}),
    ...(Object.keys(tags).length > 0 ? { tags } : {})
  };
}

export default App;
