export function useFile(fileType: string) {
  const isText = fileType.startsWith('text/');
  const isImage = fileType?.startsWith('image/');
  const isPdf = fileType === 'application/pdf';
  const isAudio = fileType?.startsWith('audio/');

  return {
    isText,
    isImage,
    isPdf,
    isAudio,
  };
}
