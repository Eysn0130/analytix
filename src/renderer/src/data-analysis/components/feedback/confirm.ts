import { emitConfirm, type ConfirmEventDetail, type ConfirmTone } from "./ConfirmCenter";

export type ConfirmOptions = Pick<
  ConfirmEventDetail,
  | "title"
  | "subtitle"
  | "description"
  | "facts"
  | "requireAcknowledgement"
  | "acknowledgementLabel"
  | "confirmLabel"
  | "cancelLabel"
  | "width"
>;

type ConfirmMethod = (options: ConfirmOptions) => Promise<boolean>;

function createConfirmMethod(tone: ConfirmTone): ConfirmMethod {
  return (options: ConfirmOptions): Promise<boolean> =>
    emitConfirm({
      ...options,
      tone
    });
}

export const confirm = Object.freeze({
  open: createConfirmMethod("default"),
  danger: createConfirmMethod("danger")
});

export type Confirm = typeof confirm;
