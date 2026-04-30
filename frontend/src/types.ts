export type Environment = "dev" | "stage" | "prod";
export type EncryptionMode = "AES256" | "aws:kms";

export interface BucketRequestPayload {
  team: string;
  environment: Environment;
  bucketName?: string;
  enableVersioning: boolean;
  encryption: EncryptionMode;
  kmsKeyArn?: string;
  tags?: Record<string, string>;
}

export interface BucketResult {
  bucketName: string;
  bucketArn: string;
  region: string;
  versioningEnabled: boolean;
  encryption: string;
  tags: Record<string, string>;
}

export interface APIErrorResponse {
  error: string;
  message: string;
  fields?: Record<string, string>;
}
