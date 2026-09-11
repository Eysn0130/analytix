import { emitPrompt, type PromptEventDetail, type PromptFieldDetail, type PromptValues } from "./PromptCenter";

type PromptBaseOptions = Pick<
  PromptEventDetail,
  "title" | "subtitle" | "description" | "layout" | "confirmLabel" | "cancelLabel" | "width"
>;

export type PromptTextOptions = PromptBaseOptions &
  Omit<PromptFieldDetail, "key" | "description" | "label"> & {
    key?: string;
    label?: string;
    fieldDescription?: string;
  };

export type PromptFormOptions = PromptBaseOptions & {
  fields: PromptFieldDetail[];
};

const DEFAULT_TEXT_KEY = "value";

async function openPrompt(options: PromptFormOptions): Promise<PromptValues | null> {
  return emitPrompt(options);
}

async function openTextPrompt(options: PromptTextOptions): Promise<string | null> {
  const key = String(options.key || DEFAULT_TEXT_KEY).trim() || DEFAULT_TEXT_KEY;
  const result = await emitPrompt({
    title: options.title,
    subtitle: options.subtitle,
    description: options.description,
    layout: options.layout,
    confirmLabel: options.confirmLabel,
    cancelLabel: options.cancelLabel,
    width: options.width,
    fields: [
      {
        key,
        label: options.label || "输入内容",
        placeholder: options.placeholder,
        defaultValue: options.defaultValue,
        description: options.fieldDescription,
        autoComplete: options.autoComplete,
        selectOnFocus: options.selectOnFocus,
        type: options.type
      }
    ]
  });

  if (!result) {
    return null;
  }

  return result[key] ?? "";
}

export const prompt = Object.freeze({
  text: openTextPrompt,
  form: openPrompt
});

export type Prompt = typeof prompt;
