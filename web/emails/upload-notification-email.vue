<script setup lang="ts">
import {
  Body,
  Container,
  Head,
  Heading,
  Hr,
  Html,
  Link,
  Preview,
  Section,
  Tailwind,
  Text,
} from '@vue-email/components';

import { tailwindConfig } from './tailwindConfig';

defineProps({
  clientName: {
    type: String,
    required: true,
  },
  fileName: {
    type: String,
    required: true,
  },
  productName: {
    type: String,
    default: 'ClientShare',
  },
});

// These are Go template actions, emitted as strings for the backend renderer.
// Keeping them out of Vue's template grammar lets build and typecheck use the
// same standard interpolation syntax without weakening template validation.
const previewText =
  '{{ if gt (len .Files) 1 }}{{ len .Files }} new files uploaded{{ else }}New file uploaded {{ index .Files 0 }}{{ end }}';
const headingText =
  '{{ if gt (len .Files) 1 }}{{ len .Files }} new files uploaded{{ else }}New file uploaded{{ end }}';
const filesStart = '{{ range .Files }}';
const filesEnd = '{{ end }}';
const uploadedFileURL = '{{ $.BaseURL }}/files/{{ .ID }}';
const uploadedFileName = '{{ .Filename }}';
</script>

<template>
  <Html lang="en">
    <Head />
    <Preview>{{ previewText }}</Preview>

    <Tailwind :config="tailwindConfig">
      <Body class="m-0 bg-surface-bg px-0 py-6 font-sans text-text-primary">
        <Container class="mx-auto w-full max-w-160 px-4">
          <Text class="m-0 mb-4 text-xs font-semibold uppercase tracking-wide text-text-muted">
            {{ productName }}
          </Text>

          <Section
            class="rounded-xl border border-surface-border bg-surface-card px-6 py-6 shadow-card"
          >
            <Heading as="h1" class="m-0 mb-3 text-2xl font-semibold text-text-primary">
              {{ headingText }}
            </Heading>

            <Text class="m-0 mb-3 text-sm text-text-primary">
              A new file has been uploaded for client <strong>{{ clientName }}</strong
              >.
            </Text>

            {{ filesStart }}
            <li>
              <Text class="m-0 mb-3 text-sm text-text-primary">
                <strong>File:</strong> <Link :href="uploadedFileURL">{{ uploadedFileName }}</Link>
              </Text>
            </li>
            {{ filesEnd }}

            <Hr class="my-4 border-surface-border" />

            <Text class="m-0 text-xs text-text-muted">
              This is an automated notification from {{ productName }}.
            </Text>
          </Section>
        </Container>
      </Body>
    </Tailwind>
  </Html>
</template>
