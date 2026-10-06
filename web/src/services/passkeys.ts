import axios from 'axios';

export type Passkey = {
  id: string;
  name: string;
  created_at: string;
  last_used_at?: string;
};

type BeginPasskeyLoginResponse = {
  passkey: boolean;
  redirect_url?: string;
  challenge_id?: string;
  public_key?: PublicKeyCredentialRequestOptionsJSON;
};

type BeginPasskeyRegistrationResponse = {
  challenge_id: string;
  public_key: PublicKeyCredentialCreationOptionsJSON;
};

type PublicKeyCredentialDescriptorJSON = Omit<PublicKeyCredentialDescriptor, 'id'> & {
  id: string;
};

type PublicKeyCredentialRequestOptionsJSON = Omit<
  PublicKeyCredentialRequestOptions,
  'challenge' | 'allowCredentials'
> & {
  challenge: string;
  allowCredentials?: PublicKeyCredentialDescriptorJSON[];
};

type PublicKeyCredentialCreationOptionsJSON = Omit<
  PublicKeyCredentialCreationOptions,
  'challenge' | 'user' | 'excludeCredentials'
> & {
  challenge: string;
  user: Omit<PublicKeyCredentialUserEntity, 'id'> & { id: string };
  excludeCredentials?: PublicKeyCredentialDescriptorJSON[];
};

type SerializedCredential = {
  id: string;
  rawId: string;
  type: string;
  response: Record<string, string | string[] | null>;
  clientExtensionResults: AuthenticationExtensionsClientOutputs;
  authenticatorAttachment: string | null;
};

function base64URLToBuffer(value: string): ArrayBuffer {
  const padding = '='.repeat((4 - (value.length % 4)) % 4);
  const base64 = value.replace(/-/g, '+').replace(/_/g, '/') + padding;
  const binary = window.atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index);
  }
  return bytes.buffer;
}

function bufferToBase64URL(value: ArrayBuffer): string {
  const bytes = new Uint8Array(value);
  let binary = '';
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return window.btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

function credentialDescriptors(
  descriptors: PublicKeyCredentialDescriptorJSON[] | undefined,
): PublicKeyCredentialDescriptor[] | undefined {
  return descriptors?.map((descriptor) => ({
    ...descriptor,
    id: base64URLToBuffer(descriptor.id),
  }));
}

function loginOptions(
  options: PublicKeyCredentialRequestOptionsJSON,
): PublicKeyCredentialRequestOptions {
  return {
    ...options,
    challenge: base64URLToBuffer(options.challenge),
    allowCredentials: credentialDescriptors(options.allowCredentials),
  };
}

function registrationOptions(
  options: PublicKeyCredentialCreationOptionsJSON,
): PublicKeyCredentialCreationOptions {
  return {
    ...options,
    challenge: base64URLToBuffer(options.challenge),
    user: {
      ...options.user,
      id: base64URLToBuffer(options.user.id),
    },
    excludeCredentials: credentialDescriptors(options.excludeCredentials),
  };
}

function serializeCredential(credential: PublicKeyCredential): SerializedCredential {
  const base = {
    id: credential.id,
    rawId: bufferToBase64URL(credential.rawId),
    type: credential.type,
    clientExtensionResults: credential.getClientExtensionResults(),
    authenticatorAttachment: credential.authenticatorAttachment,
  };
  const response = credential.response;

  if (response instanceof AuthenticatorAttestationResponse) {
    return {
      ...base,
      response: {
        clientDataJSON: bufferToBase64URL(response.clientDataJSON),
        attestationObject: bufferToBase64URL(response.attestationObject),
        transports: response.getTransports(),
      },
    };
  }

  if (response instanceof AuthenticatorAssertionResponse) {
    return {
      ...base,
      response: {
        clientDataJSON: bufferToBase64URL(response.clientDataJSON),
        authenticatorData: bufferToBase64URL(response.authenticatorData),
        signature: bufferToBase64URL(response.signature),
        userHandle: response.userHandle ? bufferToBase64URL(response.userHandle) : null,
      },
    };
  }

  throw new Error('This browser returned an unsupported passkey response.');
}

export function passkeysSupported(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.isSecureContext &&
    typeof window.PublicKeyCredential !== 'undefined' &&
    typeof navigator.credentials?.create === 'function' &&
    typeof navigator.credentials?.get === 'function'
  );
}

export function passkeyErrorMessage(error: unknown, fallback: string): string {
  if (axios.isAxiosError(error) && typeof error.response?.data?.error === 'string') {
    return error.response.data.error;
  }
  if (error instanceof Error && error.name !== 'NotAllowedError') {
    return error.message;
  }
  return fallback;
}

export async function beginPasskeySignIn(email: string): Promise<BeginPasskeyLoginResponse> {
  const response = await axios.post<BeginPasskeyLoginResponse>('/api/auth/passkeys/login/options', {
    email,
  });
  return response.data;
}

export async function getPasskeyAssertion(
  options: PublicKeyCredentialRequestOptionsJSON,
): Promise<SerializedCredential> {
  const credential = await navigator.credentials.get({ publicKey: loginOptions(options) });
  if (!(credential instanceof PublicKeyCredential)) {
    throw new Error('No passkey was selected.');
  }
  return serializeCredential(credential);
}

export async function registerPasskey(name = ''): Promise<Passkey> {
  if (!passkeysSupported()) {
    throw new Error('Passkeys require a secure connection and a compatible browser.');
  }
  const optionsResponse = await axios.post<BeginPasskeyRegistrationResponse>(
    '/api/auth/passkeys/registration/options',
  );
  const credential = await navigator.credentials.create({
    publicKey: registrationOptions(optionsResponse.data.public_key),
  });
  if (!(credential instanceof PublicKeyCredential)) {
    throw new Error('No passkey was created.');
  }
  const response = await axios.post<Passkey>('/api/auth/passkeys/registration/verify', {
    challenge_id: optionsResponse.data.challenge_id,
    credential: serializeCredential(credential),
    name,
  });
  return response.data;
}

export async function listPasskeys(): Promise<Passkey[]> {
  const response = await axios.get<Passkey[]>('/api/auth/passkeys');
  return response.data;
}

export async function renamePasskey(id: string, name: string): Promise<Passkey> {
  const response = await axios.patch<Passkey>(`/api/auth/passkeys/${id}`, { name });
  return response.data;
}

export async function deletePasskey(id: string): Promise<void> {
  await axios.delete(`/api/auth/passkeys/${id}`);
}

export async function dismissPasskeyPrompt(): Promise<void> {
  await axios.post('/api/auth/passkeys/prompt-dismiss');
}
