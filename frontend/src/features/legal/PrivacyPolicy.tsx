import { type JSX, onMount } from "solid-js";

import { Button } from "~/ui";

export default function PrivacyPolicy(): JSX.Element {
  onMount(() => {
    document.title = "Privacy Policy - CodeForge";
  });

  return (
    <div class="flex min-h-screen items-center justify-center bg-cf-bg-primary">
      <div class="w-full max-w-2xl rounded-lg border border-cf-border bg-cf-bg-surface p-8">
        <h1 class="mb-6 text-2xl font-bold text-cf-text-primary">Privacy Policy</h1>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Data Controller</h2>
          <p class="text-sm text-cf-text-secondary">
            This CodeForge instance is self-hosted. The operator of this instance is the data
            controller responsible for your personal data under GDPR Art. 4(7).
          </p>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Data We Collect</h2>
          <ul class="list-inside list-disc space-y-1 text-sm text-cf-text-secondary">
            <li>Account information (email address, name, role)</li>
            <li>Authentication tokens and session data</li>
            <li>Project and repository metadata</li>
            <li>Conversation history with AI agents</li>
            <li>Usage metrics and cost tracking data</li>
          </ul>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Data Protection Officer</h2>
          <p class="text-sm text-cf-text-secondary">
            Contact the instance operator's Data Protection Officer for privacy inquiries.
            Self-hosted operators should configure DPO contact details in the instance settings.
          </p>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Purpose &amp; Legal Basis</h2>
          <ul class="list-inside list-disc space-y-1 text-sm text-cf-text-secondary">
            <li>
              Service delivery (account, projects, conversations) -- GDPR Art. 6(1)(b) contract
            </li>
            <li>
              External LLM processing (prompts sent to providers) -- GDPR Art. 6(1)(a) consent
            </li>
            <li>
              System security (audit logs, rate limiting) -- GDPR Art. 6(1)(f) legitimate interest
            </li>
            <li>Cost tracking and billing -- GDPR Art. 6(1)(b) contract</li>
          </ul>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Subprocessors</h2>
          <p class="mb-2 text-sm text-cf-text-secondary">
            When external LLM providers are configured, user prompts and code context may be
            transmitted to these subprocessors. Local models process data entirely on-premises.
          </p>
          <ul class="list-inside list-disc space-y-1 text-sm text-cf-text-secondary">
            <li>OpenAI (Microsoft) -- LLM inference, US (EU DPA + SCCs)</li>
            <li>Anthropic -- LLM inference, US (EU DPA + SCCs)</li>
            <li>Google (Vertex AI) -- LLM inference, EU (Frankfurt)</li>
            <li>Ollama / LM Studio -- local inference, no data transfer</li>
          </ul>
          <p class="mt-2 text-xs text-cf-text-muted">
            Active providers depend on instance configuration. Consent is required before external
            processing (see consent settings).
          </p>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Data Retention</h2>
          <ul class="list-inside list-disc space-y-1 text-sm text-cf-text-secondary">
            <li>Account data -- lifetime of account + 30 days after deletion</li>
            <li>Conversations -- 90 days after last activity (configurable)</li>
            <li>Audit log entries -- 2 years (action/resource preserved, PII anonymized)</li>
            <li>IP addresses in audit logs -- 180 days (per CNIL guidance)</li>
            <li>Cost/usage data -- 7 years (tax/accounting requirements)</li>
            <li>Consent records -- indefinite (proof-of-consent per GDPR Art. 7(1))</li>
          </ul>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Your Rights</h2>
          <ul class="list-inside list-disc space-y-1 text-sm text-cf-text-secondary">
            <li>
              Right of access (Art. 15) -- export your data via Settings &gt; Privacy &gt; Export
            </li>
            <li>Right to rectification (Art. 16) -- update your profile in Settings</li>
            <li>
              Right to erasure (Art. 17) -- delete your account via Settings &gt; Privacy &gt;
              Delete
            </li>
            <li>Right to data portability (Art. 20) -- JSON export of all your data</li>
            <li>Right to object (Art. 21) -- withdraw consent for external LLM processing</li>
            <li>Right to lodge a complaint with a supervisory authority (Art. 77)</li>
          </ul>
        </section>

        <div class="mt-8 text-center">
          <Button variant="ghost" onClick={() => window.history.back()}>
            Back
          </Button>
        </div>
      </div>
    </div>
  );
}
