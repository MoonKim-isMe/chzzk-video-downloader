import type { ReactNode } from 'react';

function inlineContent(value: string): ReactNode[] {
  const token = /(\*\*[^*]+\*\*|\`[^\`]+\`|\[[^\]]+\]\(https?:\/\/[^\s)]+\))/g;
  return value.split(token).filter(Boolean).map((part, index) => {
    if (part.startsWith('**') && part.endsWith('**')) {
      return <strong key={index}>{part.slice(2, -2)}</strong>;
    }
    if (part.startsWith('\`') && part.endsWith('\`')) {
      return <code key={index}>{part.slice(1, -1)}</code>;
    }
    const link = /^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)$/.exec(part);
    if (link) {
      return <a key={index} href={link[2]} target="_blank" rel="noopener noreferrer">{link[1]}</a>;
    }
    return <span key={index}>{part}</span>;
  });
}

function ReleaseNotes({ body }: { body: string }) {
  if (!body.trim()) {
    return (
      <div className="app-update-release-notes">
        <p>이번 버전의 변경 내역이 별도로 등록되지 않았습니다. 다운로드 페이지에서 확인하실 수 있습니다.</p>
      </div>
    );
  }

  const lines = body.replace(/\r\n?/g, '\n').split('\n');
  const blocks: ReactNode[] = [];
  let index = 0;

  while (index < lines.length) {
    const line = lines[index].trim();
    if (!line) {
      index++;
      continue;
    }

    if (line.startsWith('\`\`\`')) {
      const contents: string[] = [];
      index++;
      while (index < lines.length && !lines[index].trim().startsWith('\`\`\`')) {
        contents.push(lines[index++]);
      }
      if (index < lines.length) index++;
      blocks.push(<pre key={blocks.length}><code>{contents.join('\n')}</code></pre>);
      continue;
    }

    const heading = /^(#{1,6})\s+(.+)$/.exec(line);
    if (heading) {
      blocks.push(heading[1].length <= 2
        ? <h3 key={blocks.length}>{inlineContent(heading[2])}</h3>
        : <h4 key={blocks.length}>{inlineContent(heading[2])}</h4>);
      index++;
      continue;
    }

    if (/^(-{3,}|\*{3,})$/.test(line)) {
      blocks.push(<hr key={blocks.length} />);
      index++;
      continue;
    }

    const listItem = /^([-*+]|\d+\.)\s+(.+)$/.exec(line);
    if (listItem) {
      const ordered = /\d+\./.test(listItem[1]);
      const items: ReactNode[] = [];
      while (index < lines.length) {
        const match = /^\s*([-*+]|\d+\.)\s+(.+)$/.exec(lines[index]);
        if (!match || /\d+\./.test(match[1]) !== ordered) break;
        items.push(<li key={items.length}>{inlineContent(match[2])}</li>);
        index++;
      }
      blocks.push(ordered
        ? <ol key={blocks.length}>{items}</ol>
        : <ul key={blocks.length}>{items}</ul>);
      continue;
    }

    if (line.startsWith('>')) {
      const quotes: string[] = [];
      while (index < lines.length && lines[index].trim().startsWith('>')) {
        quotes.push(lines[index++].trim().replace(/^>\s?/, ''));
      }
      blocks.push(<blockquote key={blocks.length}>{inlineContent(quotes.join(' '))}</blockquote>);
      continue;
    }

    const paragraphs: string[] = [];
    while (index < lines.length) {
      const current = lines[index].trim();
      if (!current || /^(#{1,6})\s+/.test(current) ||
          current.startsWith('\`\`\`') || current.startsWith('>') ||
          /^([-*+]|\d+\.)\s+/.test(current) || /^(-{3,}|\*{3,})$/.test(current)) {
        break;
      }
      paragraphs.push(current);
      index++;
    }
    blocks.push(<p key={blocks.length}>{inlineContent(paragraphs.join(' '))}</p>);
  }

  return <div className="app-update-release-notes">{blocks}</div>;
}

export default ReleaseNotes;
