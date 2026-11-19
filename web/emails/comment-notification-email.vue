<script setup lang="ts">
import { Body, Button, Container, Head, Heading, Html, Preview, Section, Tailwind, Text } from '@vue-email/components';

import { tailwindConfig } from './tailwindConfig';

defineProps<{ heading: string; fileName: string; commentUrl: string }>();
</script>

<template>
  <Html lang="en">
    <Head />
    <Preview>Open ClientShare to read the comment.</Preview>
    <Tailwind :config="tailwindConfig">
      <Body class="m-0 bg-surface-bg px-0 py-6 font-sans text-text-primary">
        <Container class="mx-auto w-full max-w-160 px-4">
          <Text class="m-0 mb-4 text-xs font-semibold uppercase tracking-wide text-text-muted">
            ClientShare
          </Text>
          <Section class="rounded-xl border border-surface-border bg-surface-card px-6 py-6 shadow-card">
            <Heading as="h1" class="m-0 mb-3 text-2xl font-semibold text-text-primary">
              {{ heading }}
            </Heading>
            <Text class="m-0 mb-5 text-sm text-text-primary">
              File: <strong>{{ fileName }}</strong>
            </Text>
            <Button
              :href="commentUrl"
              class="rounded-md bg-brand px-4 py-3 text-sm font-semibold leading-4 text-white no-underline"
            >
              View comment
            </Button>
          </Section>
        </Container>
      </Body>
    </Tailwind>
  </Html>
</template>
