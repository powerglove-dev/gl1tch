import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';

const changelog = defineCollection({
  loader: glob({ pattern: '**/*.md', base: './src/content/changelog' }),
  schema: z.object({
    version: z.string(),
    date: z.string(),
    highlights: z.array(z.string()).optional(),
    breaking: z.boolean().default(false),
  }),
});

const registry = defineCollection({
  loader: glob({ pattern: '**/*.md', base: './src/content/registry' }),
  schema: z.object({
    name: z.string(),
    description: z.string(),
    tier: z.union([z.literal(1), z.literal(2)]),
    repo: z.string().optional(),
    command: z.string().optional(),
    capabilities: z.array(z.string()),
  }),
});

const pipelines = defineCollection({
  loader: glob({ pattern: '**/*.{md,mdx}', base: './src/content/pipelines' }),
  schema: z.object({
    title: z.string(),
    description: z.string(),
    order: z.number(),
  }),
});

export const collections = { changelog, registry, pipelines };
