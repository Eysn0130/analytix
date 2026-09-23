export declare class Lazy<T> {
  constructor(creator: () => Promise<T>)
  readonly hasValue: boolean
  value: Promise<T>
}
