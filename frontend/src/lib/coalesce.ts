/**
 * Wraps a refresh that bursts of events trigger (e.g. a resource refetch):
 * it runs at most once at a time, and triggers while it runs queue one more
 * run after it. A failed run does not stop later ones.
 */
export function coalesce(run: () => unknown): () => void {
  let running = false;
  let again = false;
  const trigger = (): void => {
    if (running) {
      again = true;
      return;
    }
    running = true;
    const done = (): void => {
      running = false;
      if (again) {
        again = false;
        trigger();
      }
    };
    new Promise((resolve) => resolve(run())).then(done, done);
  };
  return trigger;
}
