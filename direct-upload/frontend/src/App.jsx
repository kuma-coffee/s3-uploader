import React, { useState } from 'react';
import './App.css';

export default function App() {
  const [file, setFile] = useState(null);
  const [loading, setLoading] = useState(false);
  const [downloadUrl, setDownloadUrl] = useState('');

  const upload = async () => {
    if (!file) {
      alert('Silakan pilih file terlebih dahulu.');
      return;
    }

    try {
      setLoading(true);
      setDownloadUrl('');

      console.log('Selected File:', file);

      // 1. Request Upload URL
      const uploadRes = await fetch(
        `http://localhost:8080/upload-url?filename=${encodeURIComponent(
          file.name
        )}`,
        {
          method: 'POST',
        }
      );

      const uploadData = await uploadRes.json();

      console.log('=== Upload URL Response ===');
      console.log(uploadData);

      // 2. Upload ke Signed URL
      const putRes = await fetch(uploadData.uploadUrl, {
        method: 'PUT',
        body: file,
      });

      console.log('=== Upload File Response ===');
      console.log({
        status: putRes.status,
        ok: putRes.ok,
        statusText: putRes.statusText,
      });

      // 3. Request Download URL
      const downloadRes = await fetch(
        `http://localhost:8080/download-url/${uploadData.objectKey}`
      );

      const downloadData = await downloadRes.json();

      console.log('=== Download URL Response ===');
      console.log(downloadData);

      setDownloadUrl(downloadData.url);

      alert('Upload berhasil!');
    } catch (err) {
      console.error('Upload Error:', err);
      alert('Upload gagal.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className='container'>
      <div className='card'>
        <h2>📤 Upload File</h2>
        <p>Pilih file yang ingin di-upload ke server.</p>

        <input
          type='file'
          onChange={(e) => {
            if (e.target.files?.length) {
              setFile(e.target.files[0]);
            }
          }}
        />

        {file && (
          <div className='file-info'>
            <strong>Nama File:</strong> {file.name}
            <br />
            <strong>Ukuran:</strong> {(file.size / 1024).toFixed(2)} KB
          </div>
        )}

        <button onClick={upload} disabled={loading}>
          {loading ? 'Uploading...' : 'Upload'}
        </button>

        {downloadUrl && (
          <div className='result'>
            <h4>Download URL</h4>

            <a href={downloadUrl} target='_blank' rel='noreferrer'>
              {downloadUrl}
            </a>
          </div>
        )}
      </div>
    </div>
  );
}
