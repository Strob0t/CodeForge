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
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Purpose &amp; Legal Basis</h2>
          <p class="text-sm text-cf-text-secondary">
            We process your data to provide the CodeForge service (GDPR Art. 6(1)(b) -- contract
            performance) and to maintain system security (GDPR Art. 6(1)(f) -- legitimate interest).
          </p>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Third-Party Services</h2>
          <p class="text-sm text-cf-text-secondary">
            When using AI features, prompts and code snippets may be sent to third-party LLM
            providers (e.g., OpenAI, Anthropic) as configured by the instance operator. Local models
            process data entirely on-premises.
          </p>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Your Rights</h2>
          <p class="text-sm text-cf-text-secondary">
            Under GDPR, you have the right to access, rectify, erase, restrict processing, and port
            your data. Contact the instance operator to exercise these rights.
          </p>
        </section>

        <section class="mb-6">
          <h2 class="mb-2 text-lg font-semibold text-cf-text-primary">Data Retention</h2>
          <p class="text-sm text-cf-text-secondary">
            Data is retained for the duration of your account. Upon account deletion, personal data
            is removed in accordance with the operator's retention policy.
          </p>
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
