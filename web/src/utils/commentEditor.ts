import { Extension, type JSONContent } from '@tiptap/core';
import Link from '@tiptap/extension-link';
import Mention, { type MentionNodeAttrs } from '@tiptap/extension-mention';
import { Markdown } from '@tiptap/markdown';
import { Plugin } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import StarterKit from '@tiptap/starter-kit';
import type { SuggestionOptions } from '@tiptap/suggestion';

export interface CommentPerson {
  action?: 'share' | 'reference_team' | 'reference_no_access';
  user_type?: 'external' | 'internal';
  previously_mentioned?: boolean;
  id: string;
  name: string;
  email?: string;
}
export type CommentMentionAttrs = MentionNodeAttrs;
export type CommentSuggestions = Omit<
  SuggestionOptions<CommentPerson, CommentMentionAttrs>,
  'editor'
>;

function plainText(nodes: JSONContent[]): string {
  return nodes.map((node) => node.text || plainText(node.content || [])).join('');
}

// Keep the existing Markdown wire/storage format. An editor mention is an
// atomic inline node, not a profile lookup, HTML blob, or permission grant.
const CommentMention = Mention.extend({
  markdownTokenName: 'commentMention',
  markdownTokenizer: {
    name: 'commentMention',
    level: 'inline',
    start: (source) => source.indexOf('[@'),
    tokenize(source, _tokens, lexer) {
      const match = /^\[@((?:\\.|[^\]\\])*)\]\(mention:([A-Za-z0-9_-]{1,64})\)/.exec(source);
      if (!match) {
        return undefined;
      }
      return {
        type: 'commentMention',
        raw: match[0],
        id: match[2],
        tokens: lexer.inlineTokens(match[1]),
      };
    },
  },
  parseMarkdown(token, helpers) {
    return helpers.createNode('mention', {
      id: token.id,
      label: plainText(helpers.parseInline(token.tokens || [])),
    });
  },
  renderMarkdown(node) {
    const label = String(node.attrs?.label || node.attrs?.id || '')
      .replace(/[\\`*_[\]<>]/g, '\\$&')
      .replace(/[\r\n]/g, ' ');
    return `[@${label}](mention:${node.attrs?.id})`;
  },
});

// Match the backend's no-raw-HTML contract for Markdown block content.
const LiteralHTML = Extension.create({
  name: 'commentLiteralHTML',
  markdownTokenName: 'html',
  parseMarkdown(token, helpers) {
    const text = helpers.createTextNode(token.raw || token.text || '');
    return token.block ? helpers.createNode('paragraph', {}, [text]) : text;
  },
});

const ImageAltText = Extension.create({
  name: 'commentImageAltText',
  markdownTokenName: 'image',
  parseMarkdown(token, helpers) {
    return token.text ? helpers.createTextNode(token.text) : [];
  },
});

export function commentExtensions(
  suggestion: CommentSuggestions,
  options: {
    displayName?: (id: string) => string | undefined;
    unavailableIds?: () => string[] | undefined;
  } = {},
) {
  return [
    StarterKit.configure({ link: false, underline: false, strike: false }),
    Link.configure({
      openOnClick: false,
      autolink: false,
      linkOnPaste: false,
      markdownLinks: true,
    }),
    CommentMention.configure({
      HTMLAttributes: { class: 'comment-mention' },
      deleteTriggerWithBackspace: true,
      renderHTML: ({ node }) => [
        'span',
        { class: 'comment-mention' },
        `@${options.displayName?.(node.attrs.id) || node.attrs.label || node.attrs.id}`,
      ],
      suggestion,
    }),
    Extension.create({
      name: 'commentMentionAvailability',
      addProseMirrorPlugins: () => [
        new Plugin({
          props: {
            decorations: ({ doc }) => {
              const ids = options.unavailableIds?.();
              if (!ids?.length) {
                return DecorationSet.empty;
              }
              const marked: Decoration[] = [];
              doc.descendants((node, pos) => {
                if (node.type.name === 'mention' && ids.includes(node.attrs.id)) {
                  marked.push(
                    Decoration.node(pos, pos + node.nodeSize, {
                      class: 'comment-mention-unavailable',
                    }),
                  );
                }
              });
              return DecorationSet.create(doc, marked);
            },
          },
        }),
      ],
    }),
    LiteralHTML,
    ImageAltText,
    Markdown.configure({ markedOptions: { gfm: false, breaks: true } }),
  ];
}
