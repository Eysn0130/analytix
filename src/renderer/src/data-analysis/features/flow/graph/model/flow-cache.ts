export class BoundedCache<T> {
  private readonly store = new Map<string, T>();
  private readonly limit: number;

  constructor(limit: number) {
    this.limit = limit;
  }

  get(key: string): T | undefined {
    const value = this.store.get(key);
    if (value === undefined) {
      return undefined;
    }
    this.store.delete(key);
    this.store.set(key, value);
    return value;
  }

  set(key: string, value: T): void {
    if (!key) {
      return;
    }
    if (this.store.has(key)) {
      this.store.delete(key);
    }
    this.store.set(key, value);
    while (this.store.size > this.limit) {
      const oldest = this.store.keys().next().value;
      if (!oldest) {
        break;
      }
      this.store.delete(oldest);
    }
  }
}
