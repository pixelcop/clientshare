<script setup>
import {
  Body,
  Button,
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
  accessType: {
    type: String,
    required: true,
  },
  linkUrl: {
    type: String,
    required: true,
  },
  expiry: {
    type: String,
    required: true,
  },
  productName: {
    type: String,
    default: 'ClientShare',
  },
});
</script>

<template>
  <Html lang="en">
    <Head />
    <Preview>New secure link from ${ productName }</Preview>

    <Tailwind :config="tailwindConfig">
      <Body class="m-0 bg-surface-bg px-0 py-6 font-sans text-text-primary">
        <Container class="mx-auto w-full max-w-160 px-4">
          <Text class="m-0 mb-4 text-xs font-semibold uppercase tracking-wide text-text-muted">
            ${ productName }
          </Text>

          <Section
            class="rounded-xl border border-surface-border bg-surface-card px-6 py-6 shadow-card"
          >
            <Heading as="h1" class="m-0 mb-3 text-2xl font-semibold text-text-primary">
              Your secure link is ready
            </Heading>

            <Text class="m-0 mb-3 text-sm text-text-primary">
              A secure link to <strong>${ clientName }</strong> has been generated.
            </Text>

            <Text class="m-0 mb-5 text-sm text-text-primary">
              Access level: <strong>${ accessType }</strong>
            </Text>

            <Section class="mb-5">
              <Button
                :href="linkUrl"
                class="rounded-md bg-brand px-4 py-3 text-sm font-semibold leading-4 text-white no-underline"
              >
                Open secure link
              </Button>
            </Section>

            <Text class="m-0 mb-3 text-xs text-text-muted">This link expires on ${ expiry }.</Text>

            <Hr class="my-4 border-surface-border" />

            <Text class="m-0 mb-2 text-xs text-text-muted">
              If the button does not work, copy and paste this URL into your browser:
            </Text>

            <Link :href="linkUrl" class="m-0 break-all text-xs font-medium text-brand-dark">
              ${ linkUrl }
            </Link>

            <Text class="m-0 mt-4 text-xs text-text-muted">
              If you were not expecting this secure link, you can ignore this email.
            </Text>
          </Section>
        </Container>
      </Body>
    </Tailwind>
  </Html>
</template>
