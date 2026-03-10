interface ContextGaugeProps {
  used: number;
  total: number;
}

export default function ContextGauge(props: ContextGaugeProps) {
  const percent = () => Math.min(100, (props.used / Math.max(props.total, 1)) * 100);
  const color = () => {
    const p = percent();
    if (p >= 90) return "bg-red-500";
    if (p >= 75) return "bg-orange-500";
    if (p >= 50) return "bg-yellow-500";
    return "bg-green-500";
  };

  return (
    <div
      class="flex items-center gap-1.5"
      title={`${props.used.toLocaleString()} / ${props.total.toLocaleString()} tokens`}
    >
      <div class="w-16 h-1.5 rounded-full bg-cf-bg-inset overflow-hidden">
        <div
          class={`h-full rounded-full transition-all ${color()}`}
          style={{ width: `${percent()}%` }}
        />
      </div>
      <span class="text-[10px] text-cf-text-muted">{Math.round(percent())}%</span>
    </div>
  );
}
