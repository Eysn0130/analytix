export type CleaningRequestChannel = "context" | "jobs";

export interface CleaningRequestAuthorityToken {
  readonly caseId: string;
  readonly caseGeneration: number;
  readonly channel: CleaningRequestChannel;
  readonly requestSequence: number;
}

export class CleaningRequestAuthority {
  private caseId = "";
  private caseGeneration = 0;
  private readonly sequences: Record<CleaningRequestChannel, number> = {
    context: 0,
    jobs: 0
  };

  bindCase(caseId: string): boolean {
    const normalized = String(caseId || "").trim();
    if (normalized === this.caseId) {
      return false;
    }
    this.caseId = normalized;
    this.caseGeneration += 1;
    this.sequences.context = 0;
    this.sequences.jobs = 0;
    return true;
  }

  issue(channel: CleaningRequestChannel): CleaningRequestAuthorityToken {
    this.sequences[channel] += 1;
    return Object.freeze({
      caseId: this.caseId,
      caseGeneration: this.caseGeneration,
      channel,
      requestSequence: this.sequences[channel]
    });
  }

  accepts(token: CleaningRequestAuthorityToken): boolean {
    return token.caseId === this.caseId
      && token.caseGeneration === this.caseGeneration
      && token.requestSequence === this.sequences[token.channel];
  }

  currentCaseId(): string {
    return this.caseId;
  }
}
