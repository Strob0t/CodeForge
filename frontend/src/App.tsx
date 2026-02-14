import type { RouteSectionProps } from "@solidjs/router";

export default function App(props: RouteSectionProps) {
  return (
    <div class="flex h-screen bg-gray-50 text-gray-900">
      {/* Sidebar placeholder */}
      <aside class="w-64 border-r border-gray-200 bg-white p-4">
        <h1 class="text-xl font-bold">CodeForge</h1>
        <p class="mt-2 text-sm text-gray-500">v0.1.0</p>
      </aside>

      {/* Main content area */}
      <main class="flex-1 overflow-auto p-6">{props.children}</main>
    </div>
  );
}
